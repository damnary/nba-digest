package core

import "time"

type GameID string

type GameStatus string

const (
	GameScheduled GameStatus = "scheduled"
	GameLive      GameStatus = "live"
	GameFinal     GameStatus = "final"
	GamePostponed GameStatus = "postponed"
)

type TeamScore struct {
	Team  Team
	Score int
}

type Game struct {
	ID       GameID
	League   League
	StartsAt time.Time
	Status   GameStatus

	Home TeamScore
	Away TeamScore

	Period int
	Clock  string

	ObservedAt time.Time
}

func (g Game) IsLive() bool  { return g.Status == GameLive }
func (g Game) IsFinal() bool { return g.Status == GameFinal }

func (g Game) IsActive() bool {
	return g.Status == GameScheduled || g.Status == GameLive
}

func (g Game) Margin() int {
	d := g.Home.Score - g.Away.Score
	if d < 0 {
		return -d
	}
	return d
}

func (g Game) Leader() (TeamScore, bool) {
	switch {
	case g.Home.Score > g.Away.Score:
		return g.Home, true
	case g.Away.Score > g.Home.Score:
		return g.Away, true
	default:
		return TeamScore{}, false
	}
}

func (g Game) Involves(code TeamCode) bool {
	return g.Home.Team.Code == code || g.Away.Team.Code == code
}
