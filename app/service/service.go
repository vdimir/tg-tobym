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
	"github.com/vdimir/tg-tobym/app/plugin"
	"github.com/vdimir/tg-tobym/app/store"
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

	HTTPRootPath string
	AppVersion   string
}

// BotService contains common application data
type BotService struct {
	MaxFailNum   int
	HTTPRootPath string

	bot       *tgbotapi.BotAPI
	cfg       *Config
	store     *store.Storage
	updates   tgbotapi.UpdatesChannel
	webSrv    *http.Server
	plugins   []plugin.PlugIn
	rootRoute chi.Router

	mainLoopDone chan (struct{})
	ctx          context.Context
	ctxCancel    context.CancelFunc

	failuresNumber int
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
		MaxFailNum: 10,

		bot:   bot,
		cfg:   cfg,
		store: store,
		plugins: []plugin.PlugIn{
			&plugin.VoteApp{
				Bot:   bot,
				Store: &plugin.VoteStore{Bkt: store.GetBucket("votes")},
			},
			&plugin.ShowVersion{
				Bot:     bot,
				Version: cfg.AppVersion,
			}},
		mainLoopDone: nil,
		ctx:          ctx,
		ctxCancel:    ctxCancel,
	}
	srv.rootRoute = srv.Routes()

	webPlugin := []struct {
		path string
		app  plugin.WebApp
	}{
		{
			path: "/notify",
			app: &plugin.NotifierApp{
				Bot:    bot,
				Store:  &plugin.NotifierStore{Bkt: store.GetBucket("notifier")},
				AppURL: cfg.WebAppURL,
			},
		},
	}

	for _, sapp := range webPlugin {
		srv.rootRoute.Mount(sapp.path, sapp.app.Routes())
		srv.plugins = append(srv.plugins, sapp.app)
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

	if s.cfg.HTTPRootPath != "" {
		http.Handle(s.cfg.HTTPRootPath, s.rootRoute)
	} else {
		http.Handle("/", s.rootRoute)
	}

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
	for _, sapp := range s.plugins {
		err = sapp.Init()
		if err != nil {
			return errors.Wrapf(err, "error inialize plugin")
		}
	}

	go s.mainLoop()

	return nil
}

// MainLoop starts handling messages, blocking
func (s *BotService) mainLoop() {
	s.mainLoopDone = make(chan struct{})

	defer func() {
		log.Printf("[INFO] closing main loop")
		close(s.mainLoopDone)
		s.mainLoopDone = nil
	}()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ERROR] Ooops! MainLoop failed: %v", r)
			s.failuresNumber++
			if s.MaxFailNum < 0 || s.failuresNumber < s.MaxFailNum {
				go s.mainLoop()
			}
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

		for _, sapp := range s.plugins {
			eventCaught, err := sapp.HandleUpdate(s.ctx, &update)
			if err != nil {
				log.Printf("[WARN] Error during handling update %v", err)
			}
			if eventCaught {
				break
			}
		}
	}
}

// Close service
func (s *BotService) Close() error {
	errs := &multierror.Error{}
	if s.cfg == nil {
		return errors.Errorf("close uninialized service")
	}

	if s.bot != nil {
		s.bot.StopReceivingUpdates()
	}

	if s.cfg.UseWebHook && s.bot != nil {
		_, err := s.bot.RemoveWebhook()
		errs = multierror.Append(errs, err)
	}
	s.bot = nil

	if s.webSrv != nil {
		err := s.webSrv.Close()
		s.webSrv = nil
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

	for _, sapp := range s.plugins {
		if err := sapp.Close(); err != nil {
			errs = multierror.Append(errs, errors.Wrapf(err, "error close plugin"))
		}
	}

	if s.store != nil {
		err := s.store.Close()
		errs = multierror.Append(errs, err)
		s.store = nil
	}

	return errs.ErrorOrNil()
}
