package live

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/damnary/nba-digest/internal/core"
	"github.com/damnary/nba-digest/internal/platform/retry"
)

type watcher struct {
	poller  *Poller
	game    core.Game
	updates chan core.Game
	log     *slog.Logger

	cursor string
	plays  []core.Play
	seen   map[int]bool
}

func (w *watcher) run(ctx context.Context) error {
	ticker := time.NewTicker(w.poller.cfg.WatchInterval)
	defer ticker.Stop()

	w.seen = make(map[int]bool)

	for {
		if err := w.poll(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Warn("poll failed", "err", err)
		}

		if w.game.IsOver() {
			return w.finish(ctx)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case game := <-w.updates:
			w.game = game
		case <-ticker.C:
		}
	}
}

func (w *watcher) poll(ctx context.Context) error {
	feed, err := w.fetchPlays(ctx)
	if err != nil {
		return err
	}
	w.merge(feed)

	events := Detect(w.game, w.plays)
	if len(events) == 0 {
		return nil
	}

	inserted, err := w.poller.store.SaveEvents(ctx, events)
	if err != nil {
		return fmt.Errorf("save events: %w", err)
	}
	if len(inserted) == 0 {
		return nil
	}

	fresh := make(map[core.EventID]bool, len(inserted))
	for _, id := range inserted {
		fresh[id] = true
	}

	for _, event := range events {
		if !fresh[event.ID] {
			continue
		}
		if err := w.poller.publisher.Publish(ctx, event); err != nil {
			return fmt.Errorf("publish %s: %w", event.ID, err)
		}
	}
	return nil
}

func (w *watcher) fetchPlays(ctx context.Context) (core.PlayFeed, error) {
	if err := w.poller.acquire(ctx); err != nil {
		return core.PlayFeed{}, err
	}
	defer w.poller.release()

	return w.poller.provider.Plays(ctx, w.game, w.cursor)
}

func (w *watcher) merge(feed core.PlayFeed) {
	if feed.Cursor != "" {
		w.cursor = feed.Cursor
	}

	added := false
	for _, play := range feed.Plays {
		if w.seen[play.Seq] {
			continue
		}
		w.seen[play.Seq] = true
		w.plays = append(w.plays, play)
		added = true
	}

	if added {
		slices.SortStableFunc(w.plays, func(a, b core.Play) int { return a.Seq - b.Seq })
	}
}

func (w *watcher) finish(ctx context.Context) error {
	if w.game.Status != core.GameFinal {
		return nil
	}

	return retry.Do(ctx, w.poller.statsRetry, func(ctx context.Context) error {
		box, err := w.fetchBoxScore(ctx)
		if err != nil {
			return fmt.Errorf("box score: %w", err)
		}

		stats := Aggregate(box, w.plays)
		if err := w.poller.store.SaveGameStats(ctx, stats); err != nil {
			return fmt.Errorf("save stats: %w", err)
		}

		w.log.Info("game finished",
			"score", fmt.Sprintf("%d:%d", w.game.Home.Score, w.game.Away.Score),
			"plays", len(w.plays),
			"clutch_margin", stats.ClutchMargin)
		return nil
	})
}

func (w *watcher) fetchBoxScore(ctx context.Context) (core.GameStats, error) {
	if err := w.poller.acquire(ctx); err != nil {
		return core.GameStats{}, err
	}
	defer w.poller.release()

	return w.poller.provider.BoxScore(ctx, w.game)
}
