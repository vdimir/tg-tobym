package main

import (
	"bufio"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"github.com/vdimir/tg-tobym/app/service"
)

var revision = "local"

// Opts contains command-line options
type Opts struct {
	Bot struct {
		Token      string
		WebHookURL string
	}
	Store struct {
		Path string
	}
}

func parseArgs() (Opts, error) {
	var opts Opts

	flag.StringVar(&opts.Bot.Token, "token", "", "path to file with token")
	flag.StringVar(&opts.Bot.WebHookURL, "webhook", "", "bot webhook url")
	flag.StringVar(&opts.Store.Path, "data_path", "./var", "folder to store data")

	flag.Parse()

	return opts, nil
}

func readToken(path string) (string, error) {
	tokenStr := os.Getenv("BOT_TOKEN")
	if tokenStr != "" {
		return tokenStr, nil
	}

	if path == "" {
		return "", errors.Errorf("You should pass token argument or set BOT_TOKEN environment variable")
	}

	tokenFile, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "Cannot open token file")
	}
	token, err := bufio.NewReader(io.LimitReader(tokenFile, 256)).ReadString('\n')
	if err != nil {
		return "", errors.Wrapf(err, "Cannot read token file")
	}

	tokenStr = strings.TrimSuffix(string(token), "\n")
	return tokenStr, nil
}

func main() {
	log.Printf("[INFO] Running version %s", revision)

	opts, err := parseArgs()
	if err != nil {
		log.Fatalf("[ERROR] Wrong arguments %v", err)
	}

	token, err := readToken(opts.Bot.Token)
	if err != nil {
		log.Fatalf("[ERROR] token reading error %v", err)
	}

	cfg := &service.Config{
		Token:      token,
		DataPath:   opts.Store.Path,
		WebHookURL: opts.Bot.WebHookURL,
		Debug:      false,
	}

	botService, err := service.NewBotService(cfg)
	if err != nil {
		log.Fatalf("[ERROR] cannot create bot %v", err)
	}

	err = botService.Init()
	if err != nil {
		log.Fatalf("[ERROR] cannot initialize bot %v", err)
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
	log.Printf("[INFO] Bye :)")
}
