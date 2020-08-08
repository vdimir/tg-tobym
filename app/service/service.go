package service

import (
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
}

// BotService contains common application data
type BotService struct {
	bot     *tgbotapi.BotAPI
	cfg     *Config
	store   *store.Storage
	updates tgbotapi.UpdatesChannel
	srv     *http.Server
	subapps []subapp.SubApp
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
		bot, err = tgbotapi.NewBotAPIWithClient(cfg.Token, cfg.BotClient)
	}
	if err != nil {
		return nil, err
	}
	bot.Debug = cfg.Debug
	srv := &BotService{
		bot:     bot,
		cfg:     cfg,
		store:   store,
		subapps: []subapp.SubApp{},
	}

	srv.subapps = append(srv.subapps, &subapp.VoteApp{
		Bot:   bot,
		Store: &subapp.VoteStore{Store: store},
	})

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
		s.srv = &http.Server{Addr: s.cfg.WebHookListen}

		go func() {
			log.Printf("[DEBUG] Start listen %q", s.srv.Addr)
			err = s.srv.ListenAndServe()
			if err != http.ErrServerClosed {
				log.Printf("[ERROR] Listen error: %v", err)
			}
		}()

		err = waitHTTPServerStart(s.srv.Addr, 10)
	} else {
		log.Printf("[INFO] Set up LongPool")
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60
		s.updates, err = s.bot.GetUpdatesChan(u)
	}
	return err
}

// MainLoop starts handling messages, blocking
func (s *BotService) MainLoop() {
	for update := range s.updates {
		for _, sapp := range s.subapps {
			cont, err := sapp.HandleUpdate(&update)
			if err != nil {
				log.Printf("[WARN] Error during handling update %v", err)
			}
			if !cont {
				break
			}
		}
	}
}

// Close service
func (s *BotService) Close() (err error) {
	errs := &multierror.Error{}
	if s.cfg == nil {
		return errors.Errorf("close uninialized service")
	}
	if s.cfg.WebHookURL != "" && s.srv != nil {
		_, err = s.bot.RemoveWebhook()
		errs = multierror.Append(errs, err)

		err = s.srv.Close()
		errs = multierror.Append(errs, err)
	} else {
		s.bot.StopReceivingUpdates()
	}
	return err
}
