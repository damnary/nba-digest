package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/damnary/nba-digest/internal/core"
)

func (s *Store) UpsertTeams(ctx context.Context, teams []core.Team) error {
	batch := &pgx.Batch{}
	for _, t := range teams {
		batch.Queue(
			`INSERT INTO teams (league, code, name) VALUES ($1, $2, $3)
			 ON CONFLICT (league, code) DO UPDATE SET name = EXCLUDED.name`,
			string(t.League), string(t.Code), t.Name,
		)
	}
	if err := s.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("upsert teams: %w", err)
	}
	return nil
}

func (s *Store) LinkTeamExternalID(ctx context.Context, provider, externalID string, team core.Team) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO team_external_ids (provider, external_id, league, code) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (provider, external_id) DO UPDATE SET league = EXCLUDED.league, code = EXCLUDED.code`,
		provider, externalID, string(team.League), string(team.Code),
	)
	if err != nil {
		return fmt.Errorf("link team external id: %w", err)
	}
	return nil
}

func (s *Store) TeamByExternalID(ctx context.Context, provider, externalID string) (core.Team, error) {
	const q = `SELECT t.league, t.code, t.name
	           FROM team_external_ids e
	           JOIN teams t ON t.league = e.league AND t.code = e.code
	           WHERE e.provider = $1 AND e.external_id = $2`

	var team core.Team
	err := s.pool.QueryRow(ctx, q, provider, externalID).Scan(&team.League, &team.Code, &team.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Team{}, fmt.Errorf("team %s/%s: %w", provider, externalID, ErrNotFound)
	}
	if err != nil {
		return core.Team{}, fmt.Errorf("query team by external id: %w", err)
	}
	return team, nil
}

func (s *Store) Teams(ctx context.Context, league core.League) ([]core.Team, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT league, code, name FROM teams WHERE league = $1 ORDER BY code`, string(league))
	if err != nil {
		return nil, fmt.Errorf("query teams: %w", err)
	}
	defer rows.Close()

	var out []core.Team
	for rows.Next() {
		var t core.Team
		if err := rows.Scan(&t.League, &t.Code, &t.Name); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate teams: %w", err)
	}
	return out, nil
}
