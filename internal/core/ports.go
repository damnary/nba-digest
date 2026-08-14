package core

import "context"

type ExternalTeam struct {
	ExternalID string
	Team       Team
}

type PlayFeed struct {
	Plays  []Play
	Cursor string
}

type ScoreProvider interface {
	Name() string
	Teams(ctx context.Context, league League) ([]ExternalTeam, error)
	Games(ctx context.Context, league League, day Day) ([]Game, error)
	Plays(ctx context.Context, game Game, cursor string) (PlayFeed, error)
	BoxScore(ctx context.Context, game Game) (GameStats, error)
}
