package digest

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

type Store interface {
	AllSubscribers(ctx context.Context) ([]core.Subscriber, error)
	SubscriptionsOf(ctx context.Context, id core.SubscriberID) ([]core.Subscription, error)
	ProcessedDigests(ctx context.Context, day core.Day) (map[core.SubscriberID]bool, error)
	MarkDigestProcessed(ctx context.Context, id core.SubscriberID, day core.Day, sent bool) (bool, error)
	GamesByDay(ctx context.Context, league core.League, day core.Day, loc *time.Location) ([]core.Game, error)
	GameStats(ctx context.Context, ids []core.GameID) (map[core.GameID]core.GameStats, error)
}

type Sender interface {
	SendDigest(ctx context.Context, chatID int64, digest core.Digest) error
}

type Scheduler struct {
	store    Store
	sender   Sender
	league   core.League
	interval time.Duration
	log      *slog.Logger
	now      func() time.Time
}

type Option func(*Scheduler)

func WithLogger(l *slog.Logger) Option {
	return func(s *Scheduler) { s.log = l }
}

func WithClock(now func() time.Time) Option {
	return func(s *Scheduler) { s.now = now }
}

func WithInterval(d time.Duration) Option {
	return func(s *Scheduler) { s.interval = d }
}

func New(store Store, sender Sender, league core.League, opts ...Option) *Scheduler {
	s := &Scheduler{
		store:    store,
		sender:   sender,
		league:   league,
		interval: time.Minute,
		log:      slog.Default(),
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		if err := s.Tick(ctx, s.now()); err != nil && ctx.Err() == nil {
			s.log.Error("digest tick failed", "err", err)
		}
	}
}

func (s *Scheduler) Tick(ctx context.Context, now time.Time) error {
	subscribers, err := s.store.AllSubscribers(ctx)
	if err != nil {
		return fmt.Errorf("subscribers: %w", err)
	}

	processed := make(map[core.Day]map[core.SubscriberID]bool)

	for _, sub := range subscribers {
		local := sub.LocalTime(now)
		if !sub.DigestAt.Due(local, core.DigestCatchUp) {
			continue
		}

		day := core.DayOf(local).Prev()
		if _, ok := processed[day]; !ok {
			done, err := s.store.ProcessedDigests(ctx, day)
			if err != nil {
				return fmt.Errorf("processed digests: %w", err)
			}
			processed[day] = done
		}
		if processed[day][sub.ID] {
			continue
		}

		if err := s.deliver(ctx, sub, day); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.log.Error("digest not delivered", "subscriber", sub.ID, "day", day, "err", err)
		}
	}
	return nil
}

func (s *Scheduler) deliver(ctx context.Context, sub core.Subscriber, day core.Day) error {
	subscriptions, err := s.store.SubscriptionsOf(ctx, sub.ID)
	if err != nil {
		return fmt.Errorf("subscriptions: %w", err)
	}

	games, err := s.store.GamesByDay(ctx, s.league, day, sub.Timezone)
	if err != nil {
		return fmt.Errorf("games: %w", err)
	}

	var ids []core.GameID
	for _, game := range games {
		if game.IsFinal() {
			ids = append(ids, game.ID)
		}
	}

	stats, err := s.store.GameStats(ctx, ids)
	if err != nil {
		return fmt.Errorf("stats: %w", err)
	}

	built := Build(sub, subscriptions, games, stats, day)

	sent := false
	if !built.IsEmpty() {
		if err := s.sender.SendDigest(ctx, sub.ChatID, built); err != nil {
			return fmt.Errorf("send: %w", err)
		}
		sent = true
	}

	if _, err := s.store.MarkDigestProcessed(ctx, sub.ID, day, sent); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}

	s.log.Info("digest processed", "subscriber", sub.ID, "day", day, "games", len(built.Games), "sent", sent)
	return nil
}
