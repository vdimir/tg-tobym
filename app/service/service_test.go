package service

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vdimir/tg-tobym/app/plugin"
)

// GetFreePort asks the kernel for a free open port that is ready to use.
// see https://github.com/phayes/freeport
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func GetPort(t *testing.T) int {
	port, err := GetFreePort()
	require.NoError(t, err)
	return port
}

type MockTelegramServer struct {
	Client       *http.Client
	SentMessages int32
}

func (m *MockTelegramServer) RoundTrip(r *http.Request) (*http.Response, error) {
	log.Printf("[INFO] Mock request %v\n", r)

	resp := &http.Response{StatusCode: http.StatusNotFound}
	setBodyOk := func(resp *http.Response, text string) {
		resp.StatusCode = http.StatusOK
		resp.Body = ioutil.NopCloser(strings.NewReader(text))
	}
	if strings.HasSuffix(r.URL.Path, "/getMe") {
		setBodyOk(resp, `{
			"ok": true,
			"result": {
				"id": 666,
				"is_bot": true,
				"first_name": "test_bot",
				"username":"test_bot",
				"can_join_groups": true,
				"can_read_all_group_messages": false,
				"supports_inline_queries": true
			}
		}`)
	}
	if strings.HasSuffix(r.URL.Path, "/setWebhook") {
		body, err := ioutil.ReadAll(r.Body)
		log.Printf("[INFO] Mock request body %v\n", string(body))
		if err != nil {
			panic(err)
		}

		defer r.Body.Close()

		if bytes.HasPrefix(body, []byte("url=")) {
			setBodyOk(resp, `{
				"ok": true,
				"result": {
					"url": "https://example.com/xxxxxx",
					"has_custom_certificate": false,
					"pending_update_count": 0,
					"max_connections": 40
				}
			}`)
		} else {
			setBodyOk(resp, `{"ok":true,"result":true,"description":"Webhook was deleted"}`)
		}
	}

	if strings.HasSuffix(r.URL.Path, "/deleteWebhook") {
		setBodyOk(resp, `{"ok":true,"result":true,"description":"Webhook was deleted"}`)
	}

	if strings.HasSuffix(r.URL.Path, "/getWebhookInfo") {
		setBodyOk(resp, `{"ok":true,"result":{"url":"","has_custom_certificate":false,"pending_update_count":0}}`)
	}

	if strings.HasSuffix(r.URL.Path, "/sendMessage") {
		setBodyOk(resp, "")
		atomic.AddInt32(&m.SentMessages, 1)
	}

	return resp, nil
}

func mockTelegramServer(t *testing.T) *MockTelegramServer {
	mock := &MockTelegramServer{
		Client: &http.Client{},
	}
	mock.Client.Transport = mock
	return mock
}

func setUp(t *testing.T, botInitFn func(*BotService)) (botService *BotService, mockTg *MockTelegramServer, tearDown func()) {
	tmpDir, err := ioutil.TempDir("", "tobym_data")
	require.NoError(t, err)

	webPort := GetPort(t)

	mockTg = mockTelegramServer(t)

	botService, err = NewBotService(&Config{
		Token:        uuid.NewV4().String(),
		DataPath:     tmpDir,
		WebAppURL:    fmt.Sprintf("https://example.com"),
		Addr:         fmt.Sprintf("127.0.0.1:%d", webPort),
		Debug:        true,
		BotClient:    mockTg.Client,
		UseWebHook:   true,
		HTTPRootPath: uuid.NewV4().String(),
	})

	require.NoError(t, err)
	if botInitFn != nil {
		botInitFn(botService)
	}

	err = botService.Init()
	require.NoError(t, err)

	tearDown = func() {
		err := botService.Close()
		_ = os.Remove(tmpDir)
		assert.NoError(t, err)
	}
	return botService, mockTg, tearDown
}

func sendVoteMsg(t *testing.T, webHookEndpoint string) {
	testMsg := `{
		"update_id": 617777777,
		"message": {
		  "message_id": 150,
		  "from": {
			"id": 199999999,
			"is_bot": false,
			"first_name": "Name",
			"username": "name",
			"language_code": "en"
		  },
		  "chat": {
			"id": -1001463780807,
			"title": "int32",
			"type": "supergroup"
		  },
		  "date": 1596902693,
		  "reply_to_message": {
			"message_id": 142,
			"from": {
			  "id": 199999999,
			  "is_bot": false,
			  "first_name": "Name",
			  "username": "name",
			  "language_code": "en"
			},
			"chat": {
			  "id": -1001463780807,
			  "title": "foobar",
			  "type": "supergroup"
			},
			"date": 1596569254,
			"text": "hello"
		  },
		  "text": "#vote",
		  "entities": [
			{
			  "offset": 0,
			  "length": 5,
			  "type": "hashtag"
			}
		  ]
		}
	}`

	resp, err := http.Post(webHookEndpoint, "application/json", strings.NewReader(testMsg))
	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
}

func TestVoteSendMsg(t *testing.T) {
	botService, mockTg, tearDown := setUp(t, nil)
	defer tearDown()
	webHookEndpoint := "http://" + botService.cfg.Addr + "/_webhook/" + botService.bot.Token
	sendVoteMsg(t, webHookEndpoint)

	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, int32(1), atomic.LoadInt32(&mockTg.SentMessages))
	assert.Equal(t, uint32(0), atomic.LoadUint32(&botService.failuresNumber))
}

type BrokenPlugin struct{ plugin.NopPlugin }

func (sapp *BrokenPlugin) HandleUpdate(_ context.Context, _ *tgbotapi.Update) (bool, error) {
	panic("broken")
}

func TestMainLoopPanic(t *testing.T) {
	botService, mockTg, tearDown := setUp(t, func(bsrv *BotService) {
		bsrv.plugins = append(bsrv.plugins, &BrokenPlugin{})
	})
	defer tearDown()

	webHookEndpoint := "http://" + botService.cfg.Addr + "/_webhook/" + botService.bot.Token
	sendVoteMsg(t, webHookEndpoint)

	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, int32(1), atomic.LoadInt32(&mockTg.SentMessages))
	assert.Equal(t, uint32(1), atomic.LoadUint32(&botService.failuresNumber))
}
