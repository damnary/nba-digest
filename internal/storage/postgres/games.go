package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/damnary/nba-digest/internal/core"
)

const gameColumns = `g.id, g.league, g.starts_at, g.status, g.period, g.clock,
	g.home_code, ht.name, g.home_score,
	g.away_code, aw.name, g.away_score,
	g.observed_at`

const gameJoins = `FROM games g
	JOIN teams ht ON ht.league = g.league AND ht.code = g.home_code
	JOIN teams aw ON aw.league = g.league AND aw.code = g.away_code`

func (s *Store) UpsertGames(ctx context.Context, games []core.Game) error {
	batch := &pgx.Batch{}
	for _, g := range games {
		batch.Queue(
			`INSERT INTO games (id, league, starts_at, status, home_code, away_code,
			                    home_score, away_score, period, clock, observed_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			 ON CONFLICT (id) DO UPDATE SET
			     status = EXCLUDED.status,
			     starts_at = EXCLUDED.starts_at,
			     home_score = EXCLUDED.home_score,
			     away_score = EXCLUDED.away_score,
			     period = EXCLUDED.period,
			     clock = EXCLUDED.clock,
			     observed_at = EXCLUDED.observed_at
			 WHERE games.observed_at <= EXCLUDED.observed_at`,
			string(g.ID), string(g.League), g.StartsAt, string(g.Status),
			string(g.Home.Team.Code), string(g.Away.Team.Code),
			g.Home.Score, g.Away.Score, g.Period, g.Clock, g.ObservedAt,
		)
	}
	if err := s.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("upsert games: %w", err)
	}
	return nil
}

func (s *Store) ActiveGames(ctx context.Context, league core.League) ([]core.Game, error) {
	q := `SELECT ` + gameColumns + ` ` + gameJoins + `
	      WHERE g.league = $1 AND g.status IN ('scheduled', 'live')
	      ORDER BY g.starts_at`

	return s.queryGames(ctx, q, string(league))
}

func (s *Store) GamesByDay(ctx context.Context, league core.League, day core.Day, loc *time.Location) ([]core.Game, error) {
	from, to := day.Bounds(loc)

	q := `SELECT ` + gameColumns + ` ` + gameJoins + `
	      WHERE g.league = $1 AND g.starts_at >= $2 AND g.starts_at < $3
	      ORDER BY g.starts_at`

	return s.queryGames(ctx, q, string(league), from, to)
}

func (s *Store) queryGames(ctx context.Context, q string, args ...any) ([]core.Game, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query games: %w", err)
	}
	defer rows.Close()

	var out []core.Game
	for rows.Next() {
		var g core.Game
		err := rows.Scan(
			&g.ID, &g.League, &g.StartsAt, &g.Status, &g.Period, &g.Clock,
			&g.Home.Team.Code, &g.Home.Team.Name, &g.Home.Score,
			&g.Away.Team.Code, &g.Away.Team.Name, &g.Away.Score,
			&g.ObservedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan game: %w", err)
		}
		g.Home.Team.League = g.League
		g.Away.Team.League = g.League
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate games: %w", err)
	}
	return out, nil
}
