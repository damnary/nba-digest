package core

import "fmt"

type League string

const (
	LeagueNBA  League = "nba"
	LeagueWNBA League = "wnba"
)

func ParseLeague(s string) (League, error) {
	switch l := League(s); l {
	case LeagueNBA, LeagueWNBA:
		return l, nil
	default:
		return "", fmt.Errorf("unknown league %q", s)
	}
}

type TeamCode string

type Team struct {
	League League
	Code   TeamCode
	Name   string
}

func (t Team) IsZero() bool { return t.Code == "" }
