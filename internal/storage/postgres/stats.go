package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/damnary/nba-digest/internal/core"
)

func (s *Store) SaveGameStats(ctx context.Context, stats core.GameStats) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		for _, l := range stats.Lines {
			_, err := tx.Exec(ctx,
				`INSERT INTO game_player_stats (game_id, player_id, player_name, team_code,
				                                points, rebounds, assists, clutch_points)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				 ON CONFLICT (game_id, player_id) DO UPDATE SET
				     player_name = EXCLUDED.player_name,
				     team_code = EXCLUDED.team_code,
				     points = EXCLUDED.points,
				     rebounds = EXCLUDED.rebounds,
				     assists = EXCLUDED.assists,
				     clutch_points = EXCLUDED.clutch_points`,
				string(stats.GameID), string(l.Player.ID), l.Player.Name, string(l.Player.Team),
				l.Points, l.Rebounds, l.Assists, l.ClutchPoints,
			)
			if err != nil {
				return fmt.Errorf("save player line %s: %w", l.Player.ID, err)
			}
		}

		_, err := tx.Exec(ctx,
			`UPDATE games SET clutch_margin = $2, stats_at = now() WHERE id = $1`,
			string(stats.GameID), stats.ClutchMargin,
		)
		if err != nil {
			return fmt.Errorf("update game stats marker: %w", err)
		}
		return nil
	})
}

func (s *Store) GameStats(ctx context.Context, ids []core.GameID) (map[core.GameID]core.GameStats, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	gameIDs := make([]string, len(ids))
	for i, id := range ids {
		gameIDs[i] = string(id)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT p.game_id, g.league, COALESCE(g.clutch_margin, 0),
		        p.player_id, p.player_name, p.team_code,
		        p.points, p.rebounds, p.assists, p.clutch_points
		 FROM game_player_stats p
		 JOIN games g ON g.id = p.game_id
		 WHERE p.game_id = ANY($1)
		 ORDER BY p.game_id, p.points DESC`, gameIDs)
	if err != nil {
		return nil, fmt.Errorf("query game stats: %w", err)
	}
	defer rows.Close()

	out := make(map[core.GameID]core.GameStats)
	for rows.Next() {
		var (
			gameID       core.GameID
			league       core.League
			clutchMargin int
			line         core.PlayerLine
		)
		err := rows.Scan(&gameID, &league, &clutchMargin,
			&line.Player.ID, &line.Player.Name, &line.Player.Team,
			&line.Points, &line.Rebounds, &line.Assists, &line.ClutchPoints)
		if err != nil {
			return nil, fmt.Errorf("scan player line: %w", err)
		}

		stats := out[gameID]
		stats.GameID = gameID
		stats.League = league
		stats.ClutchMargin = clutchMargin
		stats.Lines = append(stats.Lines, line)
		out[gameID] = stats
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate player lines: %w", err)
	}
	return out, nil
}
