package live

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

type Store interface {
	UpsertGames(ctx context.Context, games []core.Game) error
	SaveEvents(ctx context.Context, events []core.Event) ([]core.EventID, error)
	SaveGameStats(ctx context.Context, stats core.GameStats) error
}

type Publisher interface {
	Publish(ctx context.Context, event core.Event) error
}

type Config struct {
	League        core.League
	ScanInterval  time.Duration
	WatchInterval time.Duration
	MaxInflight   int
}

func (c Config) withDefaults() Config {
	if c.ScanInterval <= 0 {
		c.ScanInterval = time.Minute
	}
	if c.WatchInterval <= 0 {
		c.WatchInterval = 30 * time.Second
	}
	if c.MaxInflight <= 0 {
		c.MaxInflight = 4
	}
	return c
}

type Poller struct {
	provider  core.ScoreProvider
	store     Store
	publisher Publisher
	cfg       Config
	log       *slog.Logger
	now       func() time.Time

	sem chan struct{}
	wg  sync.WaitGroup

	mu       sync.Mutex
	watchers map[core.GameID]chan core.Game
}

func NewPoller(provider core.ScoreProvider, store Store, publisher Publisher, cfg Config, opts ...PollerOption) *Poller {
	cfg = cfg.withDefaults()

	p := &Poller{
		provider:  provider,
		store:     store,
		publisher: publisher,
		cfg:       cfg,
		log:       slog.Default(),
		now:       time.Now,
		sem:       make(chan struct{}, cfg.MaxInflight),
		watchers:  make(map[core.GameID]chan core.Game),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

type PollerOption func(*Poller)

func WithLogger(l *slog.Logger) PollerOption {
	return func(p *Poller) { p.log = l }
}

func WithClock(now func() time.Time) PollerOption {
	return func(p *Poller) { p.now = now }
}

func (p *Poller) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.cfg.ScanInterval)
	defer ticker.Stop()

	defer p.wg.Wait()

	for {
		if err := p.scan(ctx); err != nil && !errors.Is(err, context.Canceled) {
			p.log.Error("scoreboard scan failed", "league", p.cfg.League, "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (p *Poller) scan(ctx context.Context) error {
	games, err := p.fetchGames(ctx)
	if err != nil {
		return err
	}
	if len(games) == 0 {
		return nil
	}

	if err := p.store.UpsertGames(ctx, games); err != nil {
		return err
	}

	for _, game := range games {
		p.track(ctx, game)
	}
	return nil
}

func (p *Poller) fetchGames(ctx context.Context) ([]core.Game, error) {
	today := core.DayOf(p.now().UTC())

	seen := make(map[core.GameID]bool)
	var out []core.Game

	for _, day := range []core.Day{today.Prev(), today} {
		games, err := p.fetchDay(ctx, day)
		if err != nil {
			return nil, err
		}
		for _, game := range games {
			if seen[game.ID] {
				continue
			}
			seen[game.ID] = true
			out = append(out, game)
		}
	}
	return out, nil
}

func (p *Poller) fetchDay(ctx context.Context, day core.Day) ([]core.Game, error) {
	if err := p.acquire(ctx); err != nil {
		return nil, err
	}
	defer p.release()

	return p.provider.Games(ctx, p.cfg.League, day)
}

func (p *Poller) track(ctx context.Context, game core.Game) {
	p.mu.Lock()
	updates, running := p.watchers[game.ID]
	if !running {
		if !game.IsActive() {
			p.mu.Unlock()
			return
		}
		updates = make(chan core.Game, 1)
		p.watchers[game.ID] = updates
	}
	p.mu.Unlock()

	if running {
		select {
		case updates <- game:
		default:
		}
		return
	}

	w := &watcher{
		poller:  p,
		game:    game,
		updates: updates,
		log:     p.log.With("game", game.ID),
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer p.forget(game.ID)

		if err := w.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			w.log.Error("watcher stopped", "err", err)
		}
	}()
}

func (p *Poller) forget(id core.GameID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.watchers, id)
}

func (p *Poller) acquire(ctx context.Context) error {
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Poller) release() { <-p.sem }

func (p *Poller) Watching() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.watchers)
}
