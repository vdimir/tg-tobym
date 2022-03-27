package plugin

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/asdine/storm/v3"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/pkg/errors"
	"github.com/tj/go-naturaldate"
)

type TimezoneConverterStore struct {
	Bkt storm.Node
}

// TimezoneConverter ...
type TimezoneConverter struct {
	NopPlugin
	Store     *TimezoneConverterStore
	Bot       *tgbotapi.BotAPI
	timezones map[int64]chatToLocation
}

type chatToLocation struct {
	ChatID       int64 `storm:"id"`
	Locations    []*time.Location
	PrimLocation *time.Location
}

func (tapp *TimezoneConverter) Init() (err error) {
	tapp.timezones = map[int64]chatToLocation{}

	// var chats []chatToLocation
	// err = tapp.Store.Bkt.All(&chats)
	// if err != nil {
	// 	return errors.Wrapf(err, "error loading locations")
	// }
	// for _, c := range chats {
	// 	tapp.timezones[c.ChatID] = c
	// }
	// log.Printf("[INFO] loaded locations for %d chats", len(chats))
	return nil
}

func (tapp *TimezoneConverter) Commands() []CommandDescription {
	return []CommandDescription{}
}

func (tapp *TimezoneConverter) HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (caught bool, err error) {
	if upd.Message != nil {
		chatID := upd.Message.Chat.ID
		if upd.Message.Command() == "set_timezones" {
			if upd.Message.CommandArguments() == "" {
				resp := tgbotapi.NewMessage(chatID, "command need arguments")
				_, err = tapp.Bot.Send(resp)
				if err != nil {
					return true, err
				}
				return true, nil
			}
			// err := tapp.Store.Bkt.Select(q.Eq("ChatID", chatID)).Delete(&chatToken{})
			// if err != nil {
			// 	log.Printf("[ERROR] error deleting locations from storage %v", err)
			// }
			tzNames := strings.Split(upd.Message.CommandArguments(), " ")
			tzs := []*time.Location{}
			for _, tzName := range tzNames {
				tz, err := time.LoadLocation(tzName)
				if err != nil {
					resp := tgbotapi.NewMessage(chatID, fmt.Sprintf("Can't find timezone '%s': %s", tzName, err.Error()))
					_, err = tapp.Bot.Send(resp)
					if err != nil {
						return true, err
					}
				}
				tzs = append(tzs, tz)
			}
			if len(tzs) > 0 {
				primTz := tzs[0]
				curTime := time.Now()
				sort.Slice(tzs, func(i, j int) bool {
					_, offi := curTime.In(tzs[i]).Zone()
					_, offj := curTime.In(tzs[j]).Zone()
					return offi < offj
				})

				tapp.timezones[chatID] = chatToLocation{chatID, tzs, primTz}
			}
			// err = tapp.Store.Bkt.Save(&chatToLocation{chatID, tzs})
			// if err != nil {
			// 	log.Printf("[ERROR] error saving location to storage %v", err)
			// }
			log.Printf("[INFO] set %d locations for chat", len(tzs))
			resp := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ok, set %d locations", len(tzs)))
			_, err = tapp.Bot.Send(resp)
			if err != nil {
				return true, err
			}
		}

		if upd.Message.Command() == "time" {
			if tzs, has := tapp.timezones[chatID]; has && len(tzs.Locations) > 1 {
				textLines := []string{}

				args := upd.Message.CommandArguments()
				var d time.Time
				var err error
				if args == "" {
					d = time.Now().In(tzs.PrimLocation)
				} else {
					d, err = naturaldate.Parse(upd.Message.CommandArguments(), time.Now().In(tzs.PrimLocation))
					if err != nil {
						resp := tgbotapi.NewMessage(chatID, fmt.Sprintf("Can't find time '%s'", err))
						_, err = tapp.Bot.Send(resp)
						if err != nil {
							return true, err
						}
					}
				}
				for _, tz := range tzs.Locations {
					tf := fmt.Sprintf("15:04 | <i>%s (-07)</i>", tz)
					textLines = append(textLines, d.In(tz).Format(tf))
				}
				resp := tgbotapi.NewMessage(chatID, strings.Join(textLines, "\n"))
				resp.ParseMode = tgbotapi.ModeHTML

				_, err = tapp.Bot.Send(resp)
				if err != nil {
					return true, errors.Wrapf(err, "error send message")
				}
			}
			return true, nil
		}
	}
	return false, nil
}
