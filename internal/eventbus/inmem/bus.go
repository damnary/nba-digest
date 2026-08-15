package inmem

import (
	"context"
	"sync"

	"github.com/damnary/nba-digest/internal/core"
)

const DefaultBuffer = 256

type Bus struct {
	events   chan core.Event
	closeOne sync.Once
	dropped  int64
	mu       sync.Mutex
}

func New(buffer int) *Bus {
	if buffer <= 0 {
		buffer = DefaultBuffer
	}
	return &Bus{events: make(chan core.Event, buffer)}
}

func (b *Bus) Publish(ctx context.Context, event core.Event) error {
	select {
	case b.events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Bus) Close() {
	b.closeOne.Do(func() { close(b.events) })
}

func (b *Bus) Consume(ctx context.Context, handle func(context.Context, core.Event) error) error {
	for event := range b.events {
		if err := handle(ctx, event); err != nil {
			b.countDropped()
		}
	}
	return nil
}

func (b *Bus) Dropped() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

func (b *Bus) countDropped() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dropped++
}

func (b *Bus) Len() int { return len(b.events) }
