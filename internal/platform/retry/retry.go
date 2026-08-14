package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

type Policy struct {
	Attempts int
	Base     time.Duration
	Max      time.Duration
}

var Default = Policy{Attempts: 4, Base: 200 * time.Millisecond, Max: 5 * time.Second}

func (p Policy) attempts() int {
	if p.Attempts < 1 {
		return 1
	}
	return p.Attempts
}

func (p Policy) Backoff(attempt int) time.Duration {
	if p.Base <= 0 {
		return 0
	}

	d := p.Base
	for range attempt {
		d *= 2
		if p.Max > 0 && d >= p.Max {
			d = p.Max
			break
		}
	}

	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

func Do(ctx context.Context, p Policy, fn func(context.Context) error) error {
	var err error

	for attempt := range p.attempts() {
		if err = fn(ctx); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		}
		if attempt == p.attempts()-1 {
			break
		}

		select {
		case <-time.After(p.Backoff(attempt)):
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		}
	}
	return fmt.Errorf("after %d attempts: %w", p.attempts(), err)
}
