package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/damnary/nba-digest/internal/core"
)

func subscriberColumns(p string) string {
	return p + "id, " + p + "chat_id, " + p + "timezone, to_char(" + p + "digest_at, 'HH24:MI'), " +
		p + "alerts_on, " + p + "created_at"
}

func (s *Store) EnsureSubscriber(ctx context.Context, chatID int64) (core.Subscriber, error) {
	q := `INSERT INTO subscribers (chat_id) VALUES ($1)
	      ON CONFLICT (chat_id) DO UPDATE SET chat_id = EXCLUDED.chat_id
	      RETURNING ` + subscriberColumns("")

	sub, err := scanSubscriber(s.pool.QueryRow(ctx, q, chatID))
	if err != nil {
		return core.Subscriber{}, fmt.Errorf("ensure subscriber %d: %w", chatID, err)
	}
	return sub, nil
}

func (s *Store) SubscriberByChat(ctx context.Context, chatID int64) (core.Subscriber, error) {
	q := `SELECT ` + subscriberColumns("") + ` FROM subscribers WHERE chat_id = $1`

	sub, err := scanSubscriber(s.pool.QueryRow(ctx, q, chatID))
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Subscriber{}, fmt.Errorf("subscriber %d: %w", chatID, ErrNotFound)
	}
	if err != nil {
		return core.Subscriber{}, fmt.Errorf("query subscriber %d: %w", chatID, err)
	}
	return sub, nil
}

func (s *Store) AllSubscribers(ctx context.Context) ([]core.Subscriber, error) {
	q := `SELECT ` + subscriberColumns("") + ` FROM subscribers ORDER BY id`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query subscribers: %w", err)
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

func (s *Store) SetTimezone(ctx context.Context, id core.SubscriberID, loc *time.Location) error {
	_, err := s.pool.Exec(ctx, `UPDATE subscribers SET timezone = $2 WHERE id = $1`, int64(id), loc.String())
	if err != nil {
		return fmt.Errorf("set timezone: %w", err)
	}
	return nil
}

func (s *Store) SetDigestAt(ctx context.Context, id core.SubscriberID, at core.DailyTime) error {
	_, err := s.pool.Exec(ctx, `UPDATE subscribers SET digest_at = $2::time WHERE id = $1`, int64(id), at.String())
	if err != nil {
		return fmt.Errorf("set digest time: %w", err)
	}
	return nil
}

func (s *Store) SetAlerts(ctx context.Context, id core.SubscriberID, on bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE subscribers SET alerts_on = $2 WHERE id = $1`, int64(id), on)
	if err != nil {
		return fmt.Errorf("set alerts: %w", err)
	}
	return nil
}

func scanSubscriber(row pgx.Row) (core.Subscriber, error) {
	var (
		sub      core.Subscriber
		tz       string
		digestAt string
	)
	if err := row.Scan(&sub.ID, &sub.ChatID, &tz, &digestAt, &sub.AlertsOn, &sub.CreatedAt); err != nil {
		return core.Subscriber{}, err
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return core.Subscriber{}, fmt.Errorf("load timezone %q: %w", tz, err)
	}
	sub.Timezone = loc

	sub.DigestAt, err = core.ParseDailyTime(digestAt)
	if err != nil {
		return core.Subscriber{}, err
	}
	return sub, nil
}

func (s *Store) DeleteSubscriber(ctx context.Context, id core.SubscriberID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM subscribers WHERE id = $1`, int64(id))
	if err != nil {
		return fmt.Errorf("delete subscriber: %w", err)
	}
	return nil
}
