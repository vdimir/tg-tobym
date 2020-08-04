package main

import (
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vdimir/tg-tobym/app/service"
)

func main() {

	token, err := ioutil.ReadFile(".tg_bot_token")

	if err != nil {
		log.Fatalf("error reading token: %v", err)
	}

	cfg := &service.BotConfig{
		Token:    string(token),
		DataPath: "tobym.db",
		Debug:    true,
	}
	botService, err := service.NewBotService(cfg)

	if err != nil {
		log.Fatalf("[ERROR] cannot create bot %v", err)
	}

	err = botService.Init()
	if err != nil {
		log.Fatalf("[ERROR] cannot create bot %v", err)
	}

	defer func() {
		err := botService.Close()
		if err != nil {
			log.Printf("[ERROR] cannot close bot %v", err)
		}
	}()

	go botService.MainLoop()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Printf("[INFO] Bye")
}
