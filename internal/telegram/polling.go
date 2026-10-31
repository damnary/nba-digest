package telegram

import (
	"context"
	"log/slog"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

const pollTimeout = 25 * time.Second

type Metrics interface {
	UpdatesReceived()
}

type noMetrics struct{}

func (noMetrics) UpdatesReceived() {}

type Poller struct {
	metrics Metrics
	client  *Client
	handle  core.CommandHandler
	log     *slog.Logger
	replies *replier
	offset  int64
}

type PollerOption func(*Poller)

func WithPollerMetrics(m Metrics) PollerOption {
	return func(p *Poller) {
		if m != nil {
			p.metrics = m
		}
	}
}

func WithPollerLogger(l *slog.Logger) PollerOption {
	return func(p *Poller) {
		p.log = l
		p.replies.log = l
	}
}

func NewPoller(client *Client, handle core.CommandHandler, opts ...PollerOption) *Poller {
	p := &Poller{
		client:  client,
		handle:  handle,
		log:     slog.Default(),
		replies: &replier{client: client, log: slog.Default()},
		metrics: noMetrics{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Poller) Run(ctx context.Context) error {
	if err := p.client.DeleteWebhook(ctx); err != nil {
		p.log.Warn("could not drop the webhook before polling", "err", err)
	}

	for {
		if ctx.Err() != nil {
			return nil
		}

		updates, err := p.client.GetUpdates(ctx, p.offset, pollTimeout)
		if err == nil {
			p.metrics.UpdatesReceived()
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			p.log.Warn("getUpdates failed", "err", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= p.offset {
				p.offset = update.UpdateID + 1
			}
			p.replies.dispatch(ctx, p.handle, update)
		}
	}
}
