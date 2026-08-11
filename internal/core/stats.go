package core

import (
	"cmp"
	"slices"
)

const (
	ClutchFromPeriod = 4
	ClutchMaxMargin  = 10
)

type PlayerID string

type Player struct {
	ID   PlayerID
	Name string
	Team TeamCode
}

type PlayerLine struct {
	Player       Player
	Points       int
	Rebounds     int
	Assists      int
	ClutchPoints int
}

type GameStats struct {
	GameID       GameID
	League       League
	Lines        []PlayerLine
	ClutchMargin int
}

func (s GameStats) ByTeam(code TeamCode) GameStats {
	out := GameStats{GameID: s.GameID, League: s.League}
	for _, l := range s.Lines {
		if l.Player.Team == code {
			out.Lines = append(out.Lines, l)
		}
	}
	return out
}

func (s GameStats) TopScorers(n int) []PlayerLine {
	return topBy(s.Lines, n, func(l PlayerLine) int { return l.Points })
}

func (s GameStats) TopRebounder() (PlayerLine, bool) {
	return topOne(s.Lines, func(l PlayerLine) int { return l.Rebounds })
}

func (s GameStats) TopAssister() (PlayerLine, bool) {
	return topOne(s.Lines, func(l PlayerLine) int { return l.Assists })
}

func (s GameStats) ClutchLeader() (PlayerLine, bool) {
	if s.ClutchMargin > ClutchMaxMargin {
		return PlayerLine{}, false
	}
	return topOne(s.Lines, func(l PlayerLine) int { return l.ClutchPoints })
}

func topOne(lines []PlayerLine, key func(PlayerLine) int) (PlayerLine, bool) {
	top := topBy(lines, 1, key)
	if len(top) == 0 || key(top[0]) == 0 {
		return PlayerLine{}, false
	}
	return top[0], true
}

func topBy(lines []PlayerLine, n int, key func(PlayerLine) int) []PlayerLine {
	if n <= 0 || len(lines) == 0 {
		return nil
	}
	sorted := slices.Clone(lines)
	slices.SortStableFunc(sorted, func(a, b PlayerLine) int {
		return cmp.Compare(key(b), key(a))
	})
	return sorted[:min(n, len(sorted))]
}
