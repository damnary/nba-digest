package heartbeat

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/damnary/nba-digest/internal/core"
	"github.com/damnary/nba-digest/internal/platform/metrics"
)

const staleAfter = 15 * time.Minute

type Source interface {
	Snapshot() metrics.Snapshot
}

type Notifier interface {
	SendNotice(ctx context.Context, chatID int64, text string) error
}

type Reporter struct {
	source   Source
	notifier Notifier
	chatID   int64
	at       core.DailyTime
	location *time.Location
	interval time.Duration
	log      *slog.Logger
	now      func() time.Time

	previous metrics.Snapshot
	lastDay  core.Day
}

type Option func(*Reporter)

func WithLogger(l *slog.Logger) Option {
	return func(r *Reporter) { r.log = l }
}

func WithClock(now func() time.Time) Option {
	return func(r *Reporter) { r.now = now }
}

func WithInterval(d time.Duration) Option {
	return func(r *Reporter) { r.interval = d }
}

func New(source Source, notifier Notifier, chatID int64, at core.DailyTime, loc *time.Location, opts ...Option) *Reporter {
	r := &Reporter{
		source:   source,
		notifier: notifier,
		chatID:   chatID,
		at:       at,
		location: loc,
		interval: time.Minute,
		log:      slog.Default(),
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(r)
	}
	r.previous = source.Snapshot()
	return r
}

func (r *Reporter) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		if err := r.Tick(ctx, r.now()); err != nil && ctx.Err() == nil {
			r.log.Warn("could not send the daily report", "err", err)
		}
	}
}

func (r *Reporter) Tick(ctx context.Context, now time.Time) error {
	local := now.In(r.location)
	due := r.at.LastOccurrence(local)

	if local.Sub(due) > time.Hour {
		return nil
	}
	day := core.DayOf(due)
	if day == r.lastDay {
		return nil
	}

	current := r.source.Snapshot()
	text := Format(current.Since(r.previous), current, local)

	if err := r.notifier.SendNotice(ctx, r.chatID, text); err != nil {
		return err
	}

	r.previous = current
	r.lastDay = day
	return nil
}

func Format(delta, current metrics.Snapshot, now time.Time) string {
	var b strings.Builder
	b.WriteString("<b>Сутки сервиса</b>\n")

	fmt.Fprintf(&b, "\n%s скан табло — %s", mark(current.LastScan, now), ago(current.LastScan, now))
	fmt.Fprintf(&b, "\n%s опрос телеграма — %s", mark(current.LastUpdates, now), ago(current.LastUpdates, now))

	if current.GamesWated > 0 {
		fmt.Fprintf(&b, "\n\nпод наблюдением матчей: %d", current.GamesWated)
	}

	if line := join(delta.Deliveries); line != "" {
		fmt.Fprintf(&b, "\n\n<b>Отправка</b>\n%s", line)
	}
	if line := join(delta.Events); line != "" {
		fmt.Fprintf(&b, "\n\n<b>События</b>\n%s", line)
	}
	if line := join(delta.Requests); line != "" {
		fmt.Fprintf(&b, "\n\n<b>Запросы наружу</b>\n%s", line)
	}
	return b.String()
}

func mark(at, now time.Time) string {
	if at.IsZero() || now.Sub(at) > staleAfter {
		return "✕"
	}
	return "✓"
}

func ago(at, now time.Time) string {
	if at.IsZero() {
		return "ни разу"
	}

	d := now.Sub(at).Round(time.Minute)
	switch {
	case d < time.Minute:
		return "только что"
	case d < time.Hour:
		return fmt.Sprintf("%d мин назад", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d ч назад", int(d.Hours()))
	}
}

func join(counts map[string]float64) string {
	if len(counts) == 0 {
		return ""
	}

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = fmt.Sprintf("%s — %d", k, int(counts[k]))
	}
	return strings.Join(lines, "\n")
}
