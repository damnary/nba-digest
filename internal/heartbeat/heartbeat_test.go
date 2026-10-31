package heartbeat

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
	"github.com/damnary/nba-digest/internal/platform/metrics"
)

type fakeSource struct{ snaps []metrics.Snapshot }

func (f *fakeSource) Snapshot() metrics.Snapshot {
	if len(f.snaps) == 0 {
		return metrics.Snapshot{}
	}
	s := f.snaps[0]
	if len(f.snaps) > 1 {
		f.snaps = f.snaps[1:]
	}
	return s
}

type fakeNotifier struct {
	chats []int64
	texts []string
	err   error
}

func (f *fakeNotifier) SendNotice(_ context.Context, chatID int64, text string) error {
	if f.err != nil {
		return f.err
	}
	f.chats = append(f.chats, chatID)
	f.texts = append(f.texts, text)
	return nil
}

func moscow(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestReportGoesOutOncePerDay(t *testing.T) {
	source := &fakeSource{snaps: []metrics.Snapshot{
		{Deliveries: map[string]float64{"digest/sent": 2}},
		{Deliveries: map[string]float64{"digest/sent": 9}},
	}}
	notifier := &fakeNotifier{}
	loc := moscow(t)

	r := New(source, notifier, -500, core.DailyTime{Hour: 11}, loc, WithLogger(quiet()))

	morning := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	if err := r.Tick(t.Context(), morning); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(notifier.chats) != 1 || notifier.chats[0] != -500 {
		t.Fatalf("want one report to the ops chat, got %v", notifier.chats)
	}

	if err := r.Tick(t.Context(), morning.Add(2*time.Minute)); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if len(notifier.chats) != 1 {
		t.Errorf("the report must not repeat within a day, got %d", len(notifier.chats))
	}

	if err := r.Tick(t.Context(), morning.Add(24*time.Hour)); err != nil {
		t.Fatalf("next day: %v", err)
	}
	if len(notifier.chats) != 2 {
		t.Errorf("a new day deserves a new report, got %d", len(notifier.chats))
	}
}

func TestReportStaysQuietOutsideItsHour(t *testing.T) {
	notifier := &fakeNotifier{}
	r := New(&fakeSource{}, notifier, -500, core.DailyTime{Hour: 11}, moscow(t), WithLogger(quiet()))

	evening := time.Date(2026, 11, 1, 18, 0, 0, 0, time.UTC)
	if err := r.Tick(t.Context(), evening); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(notifier.chats) != 0 {
		t.Errorf("a report hours late is noise, got %d", len(notifier.chats))
	}
}

func TestReportCountsOnlyTheLastDay(t *testing.T) {
	source := &fakeSource{snaps: []metrics.Snapshot{
		{Deliveries: map[string]float64{"digest/sent": 100}},
		{Deliveries: map[string]float64{"digest/sent": 106}},
	}}
	notifier := &fakeNotifier{}
	r := New(source, notifier, -500, core.DailyTime{Hour: 11}, moscow(t), WithLogger(quiet()))

	if err := r.Tick(t.Context(), time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(notifier.texts) != 1 {
		t.Fatal("no report")
	}
	if !strings.Contains(notifier.texts[0], "digest/sent — 6") {
		t.Errorf("counters are cumulative: the report must show the day's delta, got:\n%s", notifier.texts[0])
	}
	if strings.Contains(notifier.texts[0], "106") {
		t.Errorf("the lifetime total leaked into the daily report:\n%s", notifier.texts[0])
	}
}

func TestSilenceIsVisibleInTheReport(t *testing.T) {
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	fresh := metrics.Snapshot{LastScan: now.Add(-2 * time.Minute), LastUpdates: now.Add(-time.Minute)}
	stale := metrics.Snapshot{LastScan: now.Add(-3 * time.Hour)}

	if text := Format(fresh, fresh, now); strings.Contains(text, "✕") {
		t.Errorf("a healthy service should show no crosses:\n%s", text)
	}

	text := Format(stale, stale, now)
	if strings.Count(text, "✕") != 2 {
		t.Errorf("a stale scan and a dead poller must both be flagged:\n%s", text)
	}
	if !strings.Contains(text, "ни разу") {
		t.Errorf("a poller that never ran should say so:\n%s", text)
	}
}
