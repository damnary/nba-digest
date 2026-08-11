package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/damnary/nba-digest/internal/core"
)

func (s *Store) SaveEvents(ctx context.Context, events []core.Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, e := range events {
		teams := make([]string, len(e.Teams))
		for i, t := range e.Teams {
			teams[i] = string(t)
		}
		batch.Queue(
			`INSERT INTO game_events (id, game_id, league, kind, teams, period, clock,
			                          home_score, away_score, run_team, run_points, run_against, occurred_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			 ON CONFLICT (id) DO NOTHING`,
			string(e.ID), string(e.GameID), string(e.League), string(e.Kind), teams,
			e.Period, e.Clock, e.HomeScore, e.AwayScore,
			string(e.Run.Team), e.Run.Points, e.Run.Against, e.OccurredAt,
		)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()

	inserted := 0
	for range events {
		tag, err := results.Exec()
		if err != nil {
			return 0, fmt.Errorf("save events: %w", err)
		}
		inserted += int(tag.RowsAffected())
	}
	return inserted, nil
}

func (s *Store) Cursor(ctx context.Context, gameID core.GameID) (provider, token string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT provider, cursor_token FROM game_cursors WHERE game_id = $1`, string(gameID),
	).Scan(&provider, &token)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("query cursor for %s: %w", gameID, err)
	}
	return provider, token, nil
}

func (s *Store) SetCursor(ctx context.Context, gameID core.GameID, provider, token string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO game_cursors (game_id, provider, cursor_token) VALUES ($1, $2, $3)
		 ON CONFLICT (game_id) DO UPDATE SET
		     provider = EXCLUDED.provider,
		     cursor_token = EXCLUDED.cursor_token,
		     updated_at = now()`,
		string(gameID), provider, token,
	)
	if err != nil {
		return fmt.Errorf("set cursor for %s: %w", gameID, err)
	}
	return nil
}

func (s *Store) EventsOfGame(ctx context.Context, gameID core.GameID) ([]core.Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, game_id, league, kind, teams, period, clock,
		        home_score, away_score, run_team, run_points, run_against, occurred_at
		 FROM game_events WHERE game_id = $1 ORDER BY occurred_at, id`, string(gameID))
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
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
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return out, nil
}

func scanEvent(row pgx.Row) (core.Event, error) {
	var (
		e     core.Event
		teams []string
	)
	err := row.Scan(&e.ID, &e.GameID, &e.League, &e.Kind, &teams, &e.Period, &e.Clock,
		&e.HomeScore, &e.AwayScore, &e.Run.Team, &e.Run.Points, &e.Run.Against, &e.OccurredAt)
	if err != nil {
		return core.Event{}, fmt.Errorf("scan event: %w", err)
	}

	e.Teams = make([]core.TeamCode, len(teams))
	for i, t := range teams {
		e.Teams[i] = core.TeamCode(t)
	}
	return e, nil
}
