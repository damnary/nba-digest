package retry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var fast = Policy{Attempts: 4, Base: time.Millisecond, Max: 4 * time.Millisecond}

func TestDoSucceedsAfterTransientFailures(t *testing.T) {
	var calls int
	err := Do(t.Context(), fast, func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("boom")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	if calls != 3 {
		t.Errorf("want 3 calls, got %d", calls)
	}
}

func TestDoGivesUpAndWrapsTheLastError(t *testing.T) {
	sentinel := errors.New("still broken")
	var calls int

	err := Do(t.Context(), fast, func(context.Context) error {
		calls++
		return sentinel
	})

	if calls != fast.Attempts {
		t.Errorf("want %d calls, got %d", fast.Attempts, calls)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("cause lost: %v", err)
	}
}

func TestDoStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var calls int

	err := Do(ctx, Policy{Attempts: 5, Base: time.Hour}, func(context.Context) error {
		calls++
		cancel()
		return errors.New("boom")
	})

	if calls != 1 {
		t.Errorf("want a single call, got %d", calls)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
}

func TestBackoffGrowsAndStaysBounded(t *testing.T) {
	p := Policy{Attempts: 8, Base: 100 * time.Millisecond, Max: time.Second}

	for attempt := range 8 {
		d := p.Backoff(attempt)
		if d < 0 || d > p.Max {
			t.Fatalf("attempt %d: backoff %v out of bounds", attempt, d)
		}
	}

	seen := make(map[time.Duration]bool)
	for range 50 {
		seen[p.Backoff(3)] = true
	}
	if len(seen) < 2 {
		t.Error("backoff is not jittered")
	}
}

func TestTransportRetriesServerErrors(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := &http.Client{Transport: &Transport{Policy: fast}}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %s", resp.Status)
	}
	if hits.Load() != 3 {
		t.Errorf("want 3 attempts, got %d", hits.Load())
	}
}

func TestTransportHonoursRetryAfter(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := &http.Client{Transport: &Transport{Policy: fast}}
	start := time.Now()
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if elapsed := time.Since(start); elapsed < time.Second {
		t.Errorf("Retry-After ignored, waited only %v", elapsed)
	}
}

func TestTransportLeavesClientErrorsAlone(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &Transport{Policy: fast}}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if hits.Load() != 1 {
		t.Errorf("404 must not be retried, got %d attempts", hits.Load())
	}
}

func TestTransportDoesNotReplayRequestsWithBody(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &Transport{Policy: fast}}
	resp, err := client.Post(srv.URL, "text/plain", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	if hits.Load() != 1 {
		t.Errorf("a request with a body must not be replayed, got %d attempts", hits.Load())
	}
}
