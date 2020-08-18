package plugin

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/vdimir/tg-tobym/app/common"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type NotifierApp struct {
	Bot    *tgbotapi.BotAPI
	Store  *NotifierStore
	AppURL string
}

type handlable interface {
	Handle(pattern string, handler http.Handler)
}

func (sapp *NotifierApp) Init() error {
	if sapp.AppURL == "" {
		sapp.AppURL = "http://127.0.0.1"
	}
	return nil
}

func (sapp *NotifierApp) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", sapp.handleWeb)
	r.Get("/{parseMode}", sapp.handleWeb)
	r.Post("/", sapp.handleWeb)
	r.Post("/{parseMode}", sapp.handleWeb)
	return r
}

func (sapp *NotifierApp) handleWeb(w http.ResponseWriter, r *http.Request) {
	reqToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if reqToken == "" {
		render.Status(r, http.StatusUnauthorized)
		render.PlainText(w, r, http.StatusText(http.StatusUnauthorized))
		return
	}

	chatID := sapp.Store.FindToken(reqToken)
	if chatID == 0 {
		render.Status(r, http.StatusForbidden)
		render.PlainText(w, r, http.StatusText(http.StatusForbidden))
		return
	}

	var text string
	switch r.Method {
	case "GET":
		text = r.URL.Query().Get("text")
	case "POST":
		reqBody, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil && err != io.EOF {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, common.JSON{"error": "can't read body"})
			return
		}
		if !utf8.Valid(reqBody) {
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, common.JSON{"error": "text must be encoded in UTF-8"})
			return
		}
		text = string(reqBody)
	default:
		render.Status(r, http.StatusNotImplemented)
		render.PlainText(w, r, http.StatusText(http.StatusNotImplemented))
		return
	}

	if text == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, common.JSON{"error": "text isn't provided"})
		return
	}

	resp := tgbotapi.NewMessage(chatID, text)
	parseMode := chi.URLParam(r, "parseMode")
	switch parseMode {
	case "":
		break
	case "html":
		resp.ParseMode = tgbotapi.ModeHTML
	case "md":
		resp.ParseMode = tgbotapi.ModeMarkdownV2
	default:
		render.Status(r, http.StatusNotFound)
		render.PlainText(w, r, http.StatusText(http.StatusNotFound))
		return
	}

	_, err := sapp.Bot.Send(resp)
	if err != nil {
		render.Status(r, http.StatusServiceUnavailable)
		render.JSON(w, r, common.JSON{"error": "can't send message", "verbose": err.Error()})
		return
	}
	render.Status(r, http.StatusOK)
}

func (sapp *NotifierApp) fomatTokenMsg(token string) string {
	cmd := fmt.Sprintf(
		`echo -n "Hello 界" | curl --data-binary @- -H "Content-Type: text/plain; charset=utf-8" -H "Authorization: Bearer %s" "%s/notify"`,
		token, sapp.AppURL)
	return fmt.Sprintf("token: `%s` created.\nExample usage `%s`", token, cmd)
}

func (sapp *NotifierApp) Commands() []CommandDescription {
	return []CommandDescription{
		{
			Cmd:     "notify_token",
			Help:    "Create or revoke notify token for chat.",
			Details: "To revoke token pass 'revoke' argument with token to revoke or without to revoke all ones.",
		},
	}
}

func (sapp *NotifierApp) HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (bool, error) {
	if upd.Message == nil {
		return false, nil
	}
	msgToMe := sapp.Bot.IsMessageToMe(*upd.Message) || upd.Message.Chat.IsPrivate()
	if cmd := upd.Message.Command(); msgToMe && cmd == "notify_token" {
		if args := upd.Message.CommandArguments(); args == "" {
			token, err := sapp.generateToken(upd.Message.Chat.ID)
			if err == nil {
				resp := tgbotapi.NewMessage(upd.Message.Chat.ID, sapp.fomatTokenMsg(token))
				resp.ParseMode = tgbotapi.ModeMarkdown

				_, err := sapp.Bot.Send(resp)
				return true, err
			}
			return true, errors.Wrapf(err, "generate token error")
		} else if strings.HasPrefix(args, "revoke") {
			tokenToRevoke := strings.TrimSpace(strings.TrimPrefix(args, "revoke"))
			revokedCnt, err := sapp.revokeTokens(upd.Message.Chat.ID, tokenToRevoke)
			var resp tgbotapi.MessageConfig
			if err != nil {
				resp = tgbotapi.NewMessage(upd.Message.Chat.ID, fmt.Sprintf("Can't revoke token"))
			} else if revokedCnt == 0 {
				resp = tgbotapi.NewMessage(upd.Message.Chat.ID, fmt.Sprintf("Not found"))
			} else if revokedCnt == 1 {
				resp = tgbotapi.NewMessage(upd.Message.Chat.ID, fmt.Sprintf("Ok, token revoked"))
			} else {
				resp = tgbotapi.NewMessage(upd.Message.Chat.ID, fmt.Sprintf("Ok, all tokens revoked"))
			}
			_, err = sapp.Bot.Send(resp)
			return true, err
		}
	}
	return false, nil
}

func (sapp *NotifierApp) Close() error {
	return nil
}

func (sapp *NotifierApp) generateToken(chatID int64) (string, error) {
	rawUID := make([]byte, 8)
	rawUID = append(rawUID, uuid.NewV4().Bytes()...)
	binary.LittleEndian.PutUint64(rawUID, uint64(chatID))
	token := base64.RawURLEncoding.EncodeToString(rawUID)

	err := sapp.Store.SaveToken(chatID, token)
	return token, err
}

func (sapp *NotifierApp) revokeTokens(chatID int64, token string) (int, error) {
	return sapp.Store.RemoveTokens(chatID, token)
}
