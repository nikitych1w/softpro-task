package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nikitych1w/softpro-task/internal/config"
	"github.com/nikitych1w/softpro-task/pkg/store"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/gorilla/mux"
)

type httpserver struct {
	router *mux.Router
	logger *logrus.Logger
	store  *store.Store
	config *config.Config
	Server *http.Server
	ctx    context.Context
	url    string
}

func NewHTTPServer(cfg *config.Config, lg *logrus.Logger, store *store.Store) *httpserver {
	s := &httpserver{
		router: mux.NewRouter(),
		logger: lg,
		store:  store,
		config: cfg,
		Server: &http.Server{},
	}

	s.url = fmt.Sprintf("%s:%s", s.config.Server.Host, s.config.Server.Port)
	s.configureRouter()

	return s
}

func (s *httpserver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *httpserver) configureRouter() {
	s.router.Use(s.logRequest)
	s.router.HandleFunc("/ready", s.healthCheck()).Methods("GET")
}

func (s *httpserver) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := s.logger.WithFields(logrus.Fields{
			"remote_addr": r.RemoteAddr,
		})
		logger.Infof("started %s %s", r.Method, r.RequestURI)

		start := time.Now()
		rw := &responseWriter{w, http.StatusOK}
		next.ServeHTTP(rw, r)

		var level logrus.Level
		switch {
		case rw.code >= 500:
			level = logrus.ErrorLevel
		case rw.code >= 400:
			level = logrus.WarnLevel
		default:
			level = logrus.InfoLevel
		}
		logger.Logf(
			level,
			"completed with %d %s in %v",
			rw.code,
			http.StatusText(rw.code),
			time.Now().Sub(start),
		)
	})
}

func (s *httpserver) healthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var status int
		if err := s.store.Ping(); err != nil {
			status = http.StatusServiceUnavailable
		} else {
			status = http.StatusOK
		}
		s.respond(w, r, status, status)
	}
}

func (s *httpserver) respond(w http.ResponseWriter, _ *http.Request, code int, data interface{}) {
	w.WriteHeader(code)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func (s *httpserver) Shutdown(ctx context.Context) error {
	if err := s.Server.Shutdown(ctx); err != nil {
		return err
	}
	s.logger.Infof("		========= [HTTP server is stopping...]")

	return nil
}
