package postgres

import (
	"context"
	"fmt"

	"github.com/damnary/nba-digest/internal/core"
)

func (s *Store) AddSubscription(ctx context.Context, id core.SubscriberID, league core.League, team core.TeamCode) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO subscriptions (subscriber_id, league, team_code) VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		int64(id), string(league), string(team),
	)
	if err != nil {
		return fmt.Errorf("add subscription: %w", err)
	}
	return nil
}

func (s *Store) RemoveSubscription(ctx context.Context, id core.SubscriberID, league core.League, team core.TeamCode) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM subscriptions WHERE subscriber_id = $1 AND league = $2 AND team_code = $3`,
		int64(id), string(league), string(team),
	)
	if err != nil {
		return fmt.Errorf("remove subscription: %w", err)
	}
	return nil
}

func (s *Store) SubscriptionsOf(ctx context.Context, id core.SubscriberID) ([]core.Subscription, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT subscriber_id, league, team_code, created_at FROM subscriptions
		 WHERE subscriber_id = $1 ORDER BY league, team_code`, int64(id))
	if err != nil {
		return nil, fmt.Errorf("query subscriptions: %w", err)
	}
	defer rows.Close()

	var out []core.Subscription
	for rows.Next() {
		var sub core.Subscription
		if err := rows.Scan(&sub.SubscriberID, &sub.League, &sub.Team, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}
	return out, nil
}

func (s *Store) SubscribersForTeams(ctx context.Context, league core.League, teams []core.TeamCode) ([]core.Subscriber, error) {
	if len(teams) == 0 {
		return nil, nil
	}

	codes := make([]string, len(teams))
	for i, t := range teams {
		codes[i] = string(t)
	}

	q := `SELECT DISTINCT ` + subscriberColumns("s.") + `
	      FROM subscribers s
	      JOIN subscriptions sub ON sub.subscriber_id = s.id
	      WHERE sub.league = $1 AND sub.team_code = ANY($2)
	      ORDER BY s.id`

	rows, err := s.pool.Query(ctx, q, string(league), codes)
	if err != nil {
		return nil, fmt.Errorf("query subscribers for teams: %w", err)
	}
	defer rows.Close()

	var out []core.Subscriber
	for rows.Next() {
		sub, err := scanSubscriber(rows)
		if err != nil {
			return nil, fmt.Errorf("scan subscriber: %w", err)
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscribers: %w", err)
	}
	return out, nil
}
