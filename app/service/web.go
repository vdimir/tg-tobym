package service

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

func (s *BotService) Routes() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)

	logFmt := &middleware.DefaultLogFormatter{
		Logger: log.New(os.Stdout, "", log.LstdFlags), NoColor: true}
	loggerMiddleware := middleware.RequestLogger(logFmt)
	r.Use(loggerMiddleware)
	r.Use(middleware.Recoverer)

	r.Use(middleware.StripSlashes)

	r.Get("/robots.txt", s.handleRobotsTxt)

	return r
}

func (s *BotService) handleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	allowedPaths := []string{}
	buf := bytes.NewBufferString("User-agent: *\nDisallow: /\n")
	for _, path := range allowedPaths {
		buf.WriteString(fmt.Sprintf("Allow: %s\n", path))
	}
	buf.WriteTo(w)
}
