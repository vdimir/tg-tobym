package service

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/vdimir/tg-tobym/app/common"
	"github.com/vdimir/tg-tobym/app/store"
	"github.com/vdimir/tg-tobym/app/subapp"
)

// Config provides configuration for BotService
type Config struct {
	Token      string
	WebAppURL  string
	UseWebHook bool
	Addr       string
	Debug      bool
	DataPath   string
	BotClient  *http.Client

	AppVersion string
}

// BotService contains common application data
type BotService struct {
	bot       *tgbotapi.BotAPI
	cfg       *Config
	store     *store.Storage
	updates   tgbotapi.UpdatesChannel
	webSrv    *http.Server
	subapps   []subapp.SubApp
	rootRoute chi.Router

	mainLoopDone chan (struct{})
	ctx          context.Context
	ctxCancel    context.CancelFunc
}

// NewBotService creates BotService
func NewBotService(cfg *Config) (*BotService, error) {
	if cfg.WebAppURL == "" && cfg.UseWebHook {
		return nil, errors.Errorf("url should be set for web hook")
	}
	store, err := store.NewStorage(cfg.DataPath)
	if err != nil {
		return nil, err
	}
	var bot *tgbotapi.BotAPI
	if cfg.BotClient == nil {
		bot, err = tgbotapi.NewBotAPI(cfg.Token)
	} else {
		bot, err = tgbotapi.NewBotAPIWithClient(cfg.Token, tgbotapi.APIEndpoint, cfg.BotClient)
	}
	if err != nil {
		return nil, err
	}
	bot.Debug = cfg.Debug

	ctx, ctxCancel := context.WithCancel(context.Background())
	srv := &BotService{
		bot:   bot,
		cfg:   cfg,
		store: store,
		subapps: []subapp.SubApp{
			&subapp.VoteApp{
				Bot:   bot,
				Store: &subapp.VoteStore{Store: store},
			},
			&subapp.ShowVersion{
				Bot:     bot,
				Version: cfg.AppVersion,
			}},
		mainLoopDone: nil,
		ctx:          ctx,
		ctxCancel:    ctxCancel,
	}
	srv.rootRoute = srv.Routes()

	webSubApps := []struct {
		path string
		app  subapp.WebApp
	}{
		{
			path: "/notify",
			app: &subapp.NotifierApp{
				Bot:    bot,
				Store:  &subapp.NotifierStore{Store: store},
				AppURL: cfg.WebAppURL,
			},
		},
	}

	for _, sapp := range webSubApps {
		srv.rootRoute.Mount(sapp.path, sapp.app.Routes())
		srv.subapps = append(srv.subapps, sapp.app)
	}

	return srv, nil
}

// Init service, setup connection
func (s *BotService) Init() error {
	var err error

	if s.cfg.UseWebHook {
		log.Printf("[INFO] Set up webhook for %s", s.cfg.WebAppURL)
		webHookPath := "/_webhook/" + s.bot.Token
		webHookEndpoint := s.cfg.WebAppURL + webHookPath
		_, err := url.Parse(webHookEndpoint)
		if err != nil {
			return errors.Wrapf(err, "wrong url")
		}
		_, err = s.bot.SetWebhook(tgbotapi.NewWebhook(webHookEndpoint))
		if err != nil {
			return errors.Wrapf(err, "webHook setup error")
		}

		info, err := s.bot.GetWebhookInfo()
		if err != nil {
			return err
		}
		if info.LastErrorDate != 0 {
			log.Printf("[WARN] Telegram callback failed: %s", info.LastErrorMessage)
		}

		s.updates = s.bot.ListenForWebhook(webHookPath)

	} else {
		log.Printf("[INFO] Set up long poll")
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		s.updates, err = s.bot.GetUpdatesChan(u)
	}

	http.Handle("/", s.rootRoute)

	if s.cfg.Addr == "" {
		return errors.Errorf("Addr is not set")
	}
	s.webSrv = &http.Server{Addr: s.cfg.Addr}

	go func() {
		log.Printf("[INFO] Start listen %q", s.webSrv.Addr)
		err = s.webSrv.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Printf("[ERROR] Listen error: %v", err)
		}
	}()

	err = common.WaitHTTPServerStart(s.webSrv.Addr, 10)

	if err != nil {
		return errors.Wrapf(err, "error inialize server")
	}
	for _, sapp := range s.subapps {
		err = sapp.Init()
		if err != nil {
			return errors.Wrapf(err, "error inialize subapp")
		}
	}
	return err
}

// MainLoop starts handling messages, blocking
func (s *BotService) MainLoop() {
	s.mainLoopDone = make(chan struct{})

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ERROR] Ooops! MainLoop failed: %v", r)
		}
	}()

	for cont := true; cont; {
		var update tgbotapi.Update

		select {
		case update, cont = <-s.updates:
			if !cont {
				continue
			}
		case <-s.ctx.Done():
			cont = false
			continue
		case <-time.After(time.Second):
			continue
		}

		for _, sapp := range s.subapps {
			nextSubapp, err := sapp.HandleUpdate(s.ctx, &update)
			if err != nil {
				log.Printf("[WARN] Error during handling update %v", err)
			}
			if !nextSubapp {
				break
			}
		}
	}

	log.Printf("[INFO] closing main loop")
	close(s.mainLoopDone)
}

// Close service
func (s *BotService) Close() error {
	errs := &multierror.Error{}
	if s.cfg == nil {
		return errors.Errorf("close uninialized service")
	}
	s.bot.StopReceivingUpdates()

	if s.cfg.UseWebHook {
		_, err := s.bot.RemoveWebhook()
		errs = multierror.Append(errs, err)
	}

	if s.webSrv != nil {
		err := s.webSrv.Close()
		errs = multierror.Append(errs, err)
	}

	s.ctxCancel()

	if s.mainLoopDone != nil {
		select {
		case <-s.mainLoopDone:
			// ok
		case <-time.After(time.Second * 5):
			multierror.Append(errs, errors.Errorf("Main loop isn't finished"))
		}
	}

	for _, sapp := range s.subapps {
		if err := sapp.Close(); err != nil {
			errs = multierror.Append(errs, errors.Wrapf(err, "error close subapp"))
		}
	}

	err := s.store.Close()
	errs = multierror.Append(errs, err)

	return errs.ErrorOrNil()
}
