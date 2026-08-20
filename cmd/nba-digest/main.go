package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/damnary/nba-digest/internal/alerts"
	"github.com/damnary/nba-digest/internal/core"
	"github.com/damnary/nba-digest/internal/digest"
	"github.com/damnary/nba-digest/internal/eventbus/inmem"
	"github.com/damnary/nba-digest/internal/httpapi"
	"github.com/damnary/nba-digest/internal/live"
	"github.com/damnary/nba-digest/internal/platform/config"
	"github.com/damnary/nba-digest/internal/provider/espn"
	"github.com/damnary/nba-digest/internal/provider/replay"
	"github.com/damnary/nba-digest/internal/storage/postgres"
	"github.com/damnary/nba-digest/internal/subscription"
	"github.com/damnary/nba-digest/internal/telegram"
)

const webhookPath = "/tg/hook"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		return err
	}
	log.Info("database ready")

	provider, err := newProvider(cfg)
	if err != nil {
		return err
	}

	if err := syncTeams(ctx, provider, store, cfg.League); err != nil {
		return fmt.Errorf("sync teams: %w", err)
	}

	bot := telegram.NewClient(cfg.TelegramToken)
	bus := inmem.New(inmem.DefaultBuffer)
	service := subscription.New(store, cfg.League)

	poller := live.NewPoller(provider, store, bus, live.Config{League: cfg.League}, live.WithLogger(log))
	dispatcher := alerts.New(store, bot, bus, alerts.WithLogger(log))
	scheduler := digest.New(store, bot, cfg.League, digest.WithLogger(log))

	updates, webhook := newUpdateSource(cfg, bot, service.Handle, log)

	server := httpapi.New(httpapi.Config{
		Addr:          cfg.HTTPAddr,
		WebhookPath:   webhookPath,
		Webhook:       webhook,
		Ready:         store.Ping,
		ShutdownGrace: cfg.ShutdownGrace,
	}, httpapi.WithLogger(log))

	g, gctx := errgroup.WithContext(ctx)

	g.Go(supervise(gctx, "http server", server.Run))
	g.Go(supervise(gctx, "telegram", updates.Run))
	g.Go(supervise(gctx, "digest scheduler", scheduler.Run))
	g.Go(supervise(gctx, "alerts dispatcher", dispatcher.Run))
	g.Go(supervise(gctx, "live poller", func(ctx context.Context) error {
		defer bus.Close()
		return poller.Run(ctx)
	}))

	log.Info("nba-digest started",
		"league", cfg.League,
		"provider", provider.Name(),
		"webhook", cfg.UsesWebhook())

	if err := g.Wait(); err != nil {
		return err
	}
	log.Info("stopped cleanly")
	return nil
}

func supervise(ctx context.Context, name string, run func(context.Context) error) func() error {
	return func() error {
		err := run(ctx)
		if err == nil && ctx.Err() == nil {
			return fmt.Errorf("%s stopped while the service was still running", name)
		}
		return err
	}
}

func newProvider(cfg config.Config) (core.ScoreProvider, error) {
	if !cfg.UsesReplay() {
		return espn.New(), nil
	}
	return replay.New(os.DirFS(cfg.ReplayDir), cfg.ReplaySpeed)
}

func newUpdateSource(cfg config.Config, bot *telegram.Client, handle core.CommandHandler, log *slog.Logger) (core.UpdateSource, *telegram.Webhook) {
	if !cfg.UsesWebhook() {
		return telegram.NewPoller(bot, handle, telegram.WithPollerLogger(log)), nil
	}

	hook := telegram.NewWebhook(bot, handle,
		cfg.WebhookURL+webhookPath, cfg.WebhookSecret,
		telegram.WithWebhookLogger(log))
	return hook, hook
}

func syncTeams(ctx context.Context, provider core.ScoreProvider, store *postgres.Store, league core.League) error {
	external, err := provider.Teams(ctx, league)
	if err != nil {
		return err
	}

	teams := make([]core.Team, len(external))
	for i, e := range external {
		teams[i] = e.Team
	}
	if err := store.UpsertTeams(ctx, teams); err != nil {
		return err
	}

	for _, e := range external {
		if err := store.LinkTeamExternalID(ctx, provider.Name(), e.ExternalID, e.Team); err != nil {
			return err
		}
	}

	slog.Info("teams synced", "league", league, "count", len(teams))
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
