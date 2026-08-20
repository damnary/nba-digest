package alerts

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

type Store interface {
	SubscribersForTeams(ctx context.Context, league core.League, teams []core.TeamCode) ([]core.Subscriber, error)
	CreateDeliveries(ctx context.Context, eventID core.EventID, subs []core.SubscriberID) (int, error)
	PendingRecipients(ctx context.Context, eventID core.EventID) ([]core.Subscriber, error)
	MarkDelivery(ctx context.Context, id core.SubscriberID, eventID core.EventID, status core.DeliveryStatus) error
	EventsNeedingDelivery(ctx context.Context, since time.Time) ([]core.Event, error)
}

type Sender interface {
	SendEvent(ctx context.Context, chatID int64, event core.Event) error
}

type Consumer interface {
	Consume(ctx context.Context, handle func(context.Context, core.Event) error) error
}

type Dispatcher struct {
	store    Store
	sender   Sender
	consumer Consumer
	log      *slog.Logger
	now      func() time.Time
}

type Option func(*Dispatcher)

func WithLogger(l *slog.Logger) Option {
	return func(d *Dispatcher) { d.log = l }
}

func WithClock(now func() time.Time) Option {
	return func(d *Dispatcher) { d.now = now }
}

func New(store Store, sender Sender, consumer Consumer, opts ...Option) *Dispatcher {
	d := &Dispatcher{
		store:    store,
		sender:   sender,
		consumer: consumer,
		log:      slog.Default(),
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func (d *Dispatcher) Run(ctx context.Context) error {
	if err := d.catchUp(ctx); err != nil {
		d.log.Error("catch-up failed", "err", err)
	}
	return d.consumer.Consume(ctx, d.Dispatch)
}

func (d *Dispatcher) catchUp(ctx context.Context) error {
	events, err := d.store.EventsNeedingDelivery(ctx, d.now().Add(-core.CatchUpWindow))
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	d.log.Info("catching up on undelivered events", "count", len(events))
	for _, event := range events {
		if err := d.Dispatch(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) Dispatch(ctx context.Context, event core.Event) error {
	subs, err := d.store.SubscribersForTeams(ctx, event.League, event.Teams)
	if err != nil {
		return fmt.Errorf("find subscribers: %w", err)
	}

	interested := Interested(subs)
	if len(interested) > 0 {
		ids := make([]core.SubscriberID, len(interested))
		for i, sub := range interested {
			ids[i] = sub.ID
		}
		if _, err := d.store.CreateDeliveries(ctx, event.ID, ids); err != nil {
			return fmt.Errorf("create deliveries: %w", err)
		}
	}

	recipients, err := d.store.PendingRecipients(ctx, event.ID)
	if err != nil {
		return fmt.Errorf("pending recipients: %w", err)
	}

	stale := event.IsStale(d.now())
	for _, sub := range recipients {
		if stale {
			d.mark(ctx, sub.ID, event.ID, core.DeliverySkipped)
			continue
		}

		if err := d.sender.SendEvent(ctx, sub.ChatID, event); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			d.log.Warn("alert not delivered", "chat", sub.ChatID, "event", event.ID, "err", err)
			d.mark(ctx, sub.ID, event.ID, core.DeliveryFailed)
			continue
		}
		d.mark(ctx, sub.ID, event.ID, core.DeliverySent)
	}

	if stale && len(recipients) > 0 {
		d.log.Info("event too old to notify", "event", event.ID, "skipped", len(recipients))
	}
	return nil
}

func (d *Dispatcher) mark(ctx context.Context, id core.SubscriberID, event core.EventID, status core.DeliveryStatus) {
	if err := d.store.MarkDelivery(ctx, id, event, status); err != nil {
		d.log.Error("could not record delivery", "subscriber", id, "event", event, "err", err)
	}
}
