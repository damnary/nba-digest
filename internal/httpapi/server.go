package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

type Check func(context.Context) error

type Config struct {
	Addr          string
	WebhookPath   string
	Webhook       http.Handler
	Ready         Check
	ShutdownGrace time.Duration
}

type Server struct {
	http  *http.Server
	cfg   Config
	log   *slog.Logger
	ready Check
}

type Option func(*Server)

func WithLogger(l *slog.Logger) Option {
	return func(s *Server) { s.log = l }
}

func New(cfg Config, opts ...Option) *Server {
	if cfg.ShutdownGrace <= 0 {
		cfg.ShutdownGrace = 30 * time.Second
	}

	s := &Server{cfg: cfg, log: slog.Default(), ready: cfg.Ready}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writePlain(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", s.handleReady)

	if cfg.Webhook != nil && cfg.WebhookPath != "" {
		mux.Handle("POST "+cfg.WebhookPath, cfg.Webhook)
	}

	s.http = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Server) Handler() http.Handler { return s.http.Handler }

func (s *Server) Run(ctx context.Context) error {
	errc := make(chan error, 1)

	go func() {
		s.log.Info("http server listening", "addr", s.http.Addr)
		errc <- s.http.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err

	case <-ctx.Done():
		drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.ShutdownGrace)
		defer cancel()

		s.log.Info("http server draining", "grace", s.cfg.ShutdownGrace)
		return s.http.Shutdown(drainCtx)
	}
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.ready == nil {
		writePlain(w, http.StatusOK, "ready")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.ready(ctx); err != nil {
		s.log.Warn("readiness check failed", "err", err)
		writePlain(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	writePlain(w, http.StatusOK, "ready")
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
