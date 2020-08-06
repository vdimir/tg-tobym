package service

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

const voteCallbackDataPrefix = "vote"
const voteCallbackDataInc = voteCallbackDataPrefix + "+"
const voteCallbackDataDec = voteCallbackDataPrefix + "-"

// Config provides configuration for BotService
type Config struct {
	Token      string
	WebHookURL string
	Debug      bool
	DataPath   string
}

// BotService contains common application data
type BotService struct {
	bot     *tgbotapi.BotAPI
	cfg     *Config
	store   *Storage
	updates tgbotapi.UpdatesChannel
}

// NewBotService creates BotService
func NewBotService(cfg *Config) (*BotService, error) {
	store, err := NewStorage(cfg.DataPath)
	if err != nil {
		return nil, err
	}

	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, err
	}
	bot.Debug = cfg.Debug
	return &BotService{
		bot:   bot,
		cfg:   cfg,
		store: store,
	}, nil
}

// Init service, setup connection
func (s *BotService) Init() error {
	if s.cfg.WebHookURL != "" {
		log.Printf("[INFO] set up webhook")
		_, err := s.bot.SetWebhook(tgbotapi.NewWebhook(s.cfg.WebHookURL + s.bot.Token))
		if err != nil {
			return err
		}
		info, err := s.bot.GetWebhookInfo()
		if err != nil {
			log.Fatal(err)
		}
		if info.LastErrorDate != 0 {
			log.Printf("Telegram callback failed: %s", info.LastErrorMessage)
		}
		s.updates = s.bot.ListenForWebhook("/" + s.bot.Token)
		// go http.ListenAndServe("0.0.0.0:8443")
		return nil
	}

	log.Printf("[INFO] set up long pool")
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	var err error
	s.updates, err = s.bot.GetUpdatesChan(u)
	return err
}

// MainLoop starts handling messages, blocking
func (s *BotService) MainLoop() {
	for update := range s.updates {
		var err error

		if update.Message != nil {
			err = s.handleMessage(update.Message)
		}
		if update.CallbackQuery != nil {
			err = s.handleCallcackQuery(update.CallbackQuery)
		}
		if err != nil {
			log.Printf("[WARN] error during handling update %v", err)
		}
	}
}

func (s *BotService) isVotable(msg *tgbotapi.Message) bool {
	return msg.Photo != nil || (msg.Text == "#vote" && msg.ReplyToMessage != nil)
}

type VoteAggregateMsgInfo struct {
	Plus  int
	Minus int
}

func (info VoteAggregateMsgInfo) InlineKeyboardRow() []tgbotapi.InlineKeyboardButton {
	plusText := "+"
	if info.Plus > 0 {
		plusText = fmt.Sprintf("+%d", info.Plus)
	}

	minusText := "-"
	if info.Minus > 0 {
		minusText = fmt.Sprintf("-%d", info.Minus)
	}

	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(plusText, voteCallbackDataInc),
		tgbotapi.NewInlineKeyboardButtonData(minusText, voteCallbackDataDec),
	)
}

func (s *BotService) updateVote(msg *tgbotapi.CallbackQuery) (info VoteAggregateMsgInfo, err error) {
	voteMsgID := msg.Message.ReplyToMessage.MessageID
	increment := 0
	if msg.Data == voteCallbackDataInc {
		increment = 1
	} else {
		increment = -1
	}

	votedMsg, err := s.store.AddVote(msg.From.ID, voteMsgID, increment)

	if err != nil {
		return info, err
	}

	for _, inc := range votedMsg.Users {
		if inc > 0 {
			info.Plus += inc
		}
		if inc < 0 {
			info.Minus -= inc
		}
	}
	return info, nil
}

func (s *BotService) handleCallcackQuery(msg *tgbotapi.CallbackQuery) error {
	errs := &multierror.Error{}
	if strings.HasPrefix(msg.Data, voteCallbackDataPrefix) {
		if msg.Message.ReplyToMessage == nil {
			return errors.New("vote callback is not a reply")
		}
		voteAgg, err := s.updateVote(msg)
		if err != nil {
			return err
		}
		log.Printf("[DEBUG] inline rep %s", msg.InlineMessageID)
		replyMsg := tgbotapi.NewEditMessageReplyMarkup(
			msg.Message.Chat.ID, msg.Message.MessageID,
			tgbotapi.NewInlineKeyboardMarkup(voteAgg.InlineKeyboardRow()),
		)
		_, err = s.bot.AnswerCallbackQuery(tgbotapi.NewCallback(msg.ID, "ok"))
		errs = multierror.Append(errs, err)

		_, err = s.bot.Send(replyMsg)
		errs = multierror.Append(errs, err)
	}
	return errs.ErrorOrNil()
}

func (s *BotService) handleMessage(msg *tgbotapi.Message) (err error) {
	if s.isVotable(msg) {
		respMsg := tgbotapi.NewMessage(msg.Chat.ID, "let's vote it, guys")

		respMsg.ReplyToMessageID = msg.MessageID
		if msg.ReplyToMessage != nil {
			respMsg.ReplyToMessageID = msg.ReplyToMessage.MessageID
		}

		respMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			VoteAggregateMsgInfo{}.InlineKeyboardRow())
		_, err = s.bot.Send(respMsg)
	}
	return err
}

// Close service
func (s *BotService) Close() (err error) {
	if s.cfg.WebHookURL != "" {
		_, err = s.bot.RemoveWebhook()

	} else {
		s.bot.StopReceivingUpdates()
	}
	return err
}
