package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

func (s *Store) MarkDigestProcessed(ctx context.Context, id core.SubscriberID, day core.Day, sent bool) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO digest_runs (subscriber_id, day, sent) VALUES ($1, $2, $3)
		 ON CONFLICT (subscriber_id, day) DO NOTHING`,
		int64(id), dayToDate(day), sent,
	)
	if err != nil {
		return false, fmt.Errorf("mark digest processed: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) ProcessedDigests(ctx context.Context, day core.Day) (map[core.SubscriberID]bool, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT subscriber_id, sent FROM digest_runs WHERE day = $1`, dayToDate(day))
	if err != nil {
		return nil, fmt.Errorf("query digest runs: %w", err)
	}
	defer rows.Close()

	out := make(map[core.SubscriberID]bool)
	for rows.Next() {
		var (
			id   core.SubscriberID
			sent bool
		)
		if err := rows.Scan(&id, &sent); err != nil {
			return nil, fmt.Errorf("scan digest run: %w", err)
		}
		out[id] = sent
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate digest runs: %w", err)
	}
	return out, nil
}

func dayToDate(d core.Day) time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}
