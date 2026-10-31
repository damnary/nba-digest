package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	return rec.Body.String()
}

func TestScrapeCarriesTheLivenessClocks(t *testing.T) {
	m := New("test")
	m.now = func() time.Time { return time.Unix(1762000000, 0) }

	m.ScanSucceeded()
	m.UpdatesReceived()

	body := scrape(t, m)
	for _, want := range []string{
		`nba_digest_build_info{version="test"} 1`,
		"nba_digest_scan_last_success_timestamp_seconds 1.762e+09",
		"nba_digest_updates_last_success_timestamp_seconds 1.762e+09",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestDeliveriesAreSplitByOutcome(t *testing.T) {
	m := New("test")

	m.Delivered("digest", "sent")
	m.Delivered("digest", "sent")
	m.Delivered("digest", "unreachable")
	m.Delivered("alert", "skipped")

	body := scrape(t, m)
	for _, want := range []string{
		`nba_digest_deliveries_total{channel="digest",result="sent"} 2`,
		`nba_digest_deliveries_total{channel="digest",result="unreachable"} 1`,
		`nba_digest_deliveries_total{channel="alert",result="skipped"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
}

func TestTransportCountsCallsByHostAndStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "boom") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := New("test")
	client := &http.Client{Transport: &Transport{Metrics: m}}

	for _, path := range []string{"/ok", "/ok", "/boom"} {
		resp, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		resp.Body.Close()
	}

	body := scrape(t, m)
	if !strings.Contains(body, `status="200"} 2`) {
		t.Errorf("two successful calls are missing:\n%s", body)
	}
	if !strings.Contains(body, `status="500"} 1`) {
		t.Errorf("the failing call is missing:\n%s", body)
	}
	if !strings.Contains(body, "nba_digest_outbound_request_seconds_bucket") {
		t.Error("latency histogram is missing")
	}
}

func TestEventsAndWatchersAreReported(t *testing.T) {
	m := New("test")

	m.EventDetected("close_finish")
	m.EventDetected("run")
	m.EventDetected("run")
	m.GamesWatched(7)

	body := scrape(t, m)
	for _, want := range []string{
		`nba_digest_events_detected_total{kind="run"} 2`,
		`nba_digest_events_detected_total{kind="close_finish"} 1`,
		"nba_digest_games_watched 7",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}
