package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func TestHealthAndReadiness(t *testing.T) {
	failing := errors.New("database is down")
	var ready error

	srv := New(Config{
		Ready: func(context.Context) error { return ready },
	}, WithLogger(quiet()))

	get := func(path string) int {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}

	if code := get("/healthz"); code != http.StatusOK {
		t.Errorf("healthz = %d", code)
	}
	if code := get("/readyz"); code != http.StatusOK {
		t.Errorf("readyz with a healthy check = %d", code)
	}

	ready = failing
	if code := get("/readyz"); code != http.StatusServiceUnavailable {
		t.Errorf("readyz with a failing check = %d, want 503", code)
	}
	if code := get("/healthz"); code != http.StatusOK {
		t.Errorf("healthz must stay green while readiness fails, got %d", code)
	}
}

func TestWebhookIsMountedForPostOnly(t *testing.T) {
	var hits int
	srv := New(Config{
		WebhookPath: "/tg/hook",
		Webhook: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits++
			w.WriteHeader(http.StatusOK)
		}),
	}, WithLogger(quiet()))

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tg/hook", nil))
	if rec.Code != http.StatusOK || hits != 1 {
		t.Errorf("POST: status %d, hits %d", rec.Code, hits)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tg/hook", nil))
	if rec.Code == http.StatusOK {
		t.Error("GET on the webhook path should not reach the handler")
	}
	if hits != 1 {
		t.Errorf("handler ran %d times", hits)
	}
}

func TestRunServesAndShutsDownOnCancel(t *testing.T) {
	addr := freeAddr(t)
	srv := New(Config{Addr: addr, ShutdownGrace: time.Second}, WithLogger(quiet()))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	var resp *http.Response
	var err error
	for range 50 {
		resp, err = http.Get("http://" + addr + "/healthz")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server never came up: %v", err)
	}
	resp.Body.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}

	if _, err := http.Get("http://" + addr + "/healthz"); err == nil {
		t.Error("server still answers after shutdown")
	}
}

func TestRunReportsListenFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	srv := New(Config{Addr: l.Addr().String()}, WithLogger(quiet()))
	if err := srv.Run(t.Context()); err == nil {
		t.Error("binding a busy port should fail")
	}
}
