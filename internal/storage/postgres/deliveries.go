package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/damnary/nba-digest/internal/core"
)

func (s *Store) CreateDeliveries(ctx context.Context, eventID core.EventID, subs []core.SubscriberID) (int, error) {
	if len(subs) == 0 {
		return 0, nil
	}

	created := 0
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		for _, id := range subs {
			tag, err := tx.Exec(ctx,
				`INSERT INTO alert_deliveries (subscriber_id, event_id, status) VALUES ($1, $2, $3)
				 ON CONFLICT (subscriber_id, event_id) DO NOTHING`,
				int64(id), string(eventID), string(core.DeliveryPending),
			)
			if err != nil {
				return fmt.Errorf("create delivery for %d: %w", id, err)
			}
			created += int(tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return created, nil
}

func (s *Store) MarkDelivery(ctx context.Context, id core.SubscriberID, eventID core.EventID, status core.DeliveryStatus) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE alert_deliveries
		 SET status = $3, attempts = attempts + 1,
		     sent_at = CASE WHEN $3 = 'sent' THEN now() ELSE sent_at END
		 WHERE subscriber_id = $1 AND event_id = $2`,
		int64(id), string(eventID), string(status),
	)
	if err != nil {
		return fmt.Errorf("mark delivery: %w", err)
	}
	return nil
}

func (s *Store) PendingDeliveries(ctx context.Context, limit int) ([]core.Delivery, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT subscriber_id, event_id, status, attempts, created_at
		 FROM alert_deliveries WHERE status = 'pending'
		 ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending deliveries: %w", err)
	}
	defer rows.Close()

	var out []core.Delivery
	for rows.Next() {
		var d core.Delivery
		if err := rows.Scan(&d.SubscriberID, &d.EventID, &d.Status, &d.Attempts, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deliveries: %w", err)
	}
	return out, nil
}

func (s *Store) EventsWithoutDeliveries(ctx context.Context, since time.Time) ([]core.Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT e.id, e.game_id, e.league, e.kind, e.teams, e.period, e.clock,
		        e.home_score, e.away_score, e.run_team, e.run_points, e.run_against, e.occurred_at
		 FROM game_events e
		 LEFT JOIN alert_deliveries d ON d.event_id = e.id
		 WHERE e.created_at >= $1 AND d.event_id IS NULL
		 ORDER BY e.occurred_at`, since)
	if err != nil {
		return nil, fmt.Errorf("query undelivered events: %w", err)
	}
	defer rows.Close()

	var out []core.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate undelivered events: %w", err)
	}
	return out, nil
}
