package service

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/vdimir/tg-tobym/app/store"
	"github.com/vdimir/tg-tobym/app/subapp"
)

// Config provides configuration for BotService
type Config struct {
	Token         string
	WebHookURL    string
	WebHookListen string
	Debug         bool
	DataPath      string
	BotClient     *http.Client

	AppVersion string
}

// BotService contains common application data
type BotService struct {
	bot        *tgbotapi.BotAPI
	cfg        *Config
	store      *store.Storage
	updates    tgbotapi.UpdatesChannel
	webHookSrv *http.Server
	subapps    []subapp.SubApp

	mainLoopDone chan (struct{})
	ctx          context.Context
	ctxCancel    context.CancelFunc
}

// NewBotService creates BotService
func NewBotService(cfg *Config) (*BotService, error) {
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
		bot:          bot,
		cfg:          cfg,
		store:        store,
		subapps:      []subapp.SubApp{},
		mainLoopDone: nil,
		ctx:          ctx,
		ctxCancel:    ctxCancel,
	}

	subappConfigs := []subapp.Factory{
		&subapp.VoteAppConfig{},
		&subapp.ShowVersionConfig{Version: cfg.AppVersion},
	}

	for _, cfg := range subappConfigs {
		sapp, err := cfg.NewSubApp(bot, store)
		if err != nil {
			return srv, err
		}
		srv.subapps = append(srv.subapps, sapp)
	}

	return srv, nil
}

func waitHTTPServerStart(addr string, maxRetries int) error {
	retries := 0
	for range time.Tick(time.Second) {
		retries++
		conn, _ := net.DialTimeout("tcp", addr, time.Millisecond*10)
		if conn != nil {
			_ = conn.Close()
			return nil
		}
		if retries > maxRetries {
			return errors.Errorf("server not started")
		}
	}
	return nil
}

// Init service, setup connection
func (s *BotService) Init() error {
	var err error

	if s.cfg.WebHookURL != "" {
		log.Printf("[INFO] Set up webhook for %s", s.cfg.WebHookURL)

		webHookEndpoint := s.cfg.WebHookURL + "/" + s.bot.Token
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

		s.updates = s.bot.ListenForWebhook("/" + s.bot.Token)

		if s.cfg.WebHookListen == "" {
			return errors.Errorf("WebHookListen should be set if webHook used")
		}
		s.webHookSrv = &http.Server{Addr: s.cfg.WebHookListen}

		go func() {
			log.Printf("[INFO] Start listen %q", s.webHookSrv.Addr)
			err = s.webHookSrv.ListenAndServe()
			if err != http.ErrServerClosed {
				log.Printf("[ERROR] Listen error: %v", err)
			}
		}()

		err = waitHTTPServerStart(s.webHookSrv.Addr, 10)
	} else {
		log.Printf("[INFO] Set up LongPool")
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		s.updates, err = s.bot.GetUpdatesChan(u)
	}

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

	if s.cfg.WebHookURL != "" {
		_, err := s.bot.RemoveWebhook()
		errs = multierror.Append(errs, err)
	}

	if s.webHookSrv != nil {
		err := s.webHookSrv.Close()
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
