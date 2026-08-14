package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	RunMinPoints      = 12
	RunTolerance      = 2
	ComebackDeficit   = 8
	CloseFinishWindow = 3 * time.Minute
	CloseFinishMargin = 5
)

type Play struct {
	Seq        int
	Period     int
	Clock      string
	Team       TeamCode
	Player     Player
	Points     int
	HomeScore  int
	AwayScore  int
	OccurredAt time.Time
}

func (p Play) Margin() int {
	d := p.HomeScore - p.AwayScore
	if d < 0 {
		return -d
	}
	return d
}

func (p Play) IsClutch() bool { return p.Period >= ClutchFromPeriod }

func ParseClock(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty clock")
	}

	mins, rest, ok := strings.Cut(s, ":")
	if !ok {
		secs, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("parse clock %q: %w", s, err)
		}
		return time.Duration(secs * float64(time.Second)), nil
	}

	m, err := strconv.Atoi(mins)
	if err != nil {
		return 0, fmt.Errorf("parse clock %q: %w", s, err)
	}
	secs, err := strconv.ParseFloat(rest, 64)
	if err != nil {
		return 0, fmt.Errorf("parse clock %q: %w", s, err)
	}
	return time.Duration(m)*time.Minute + time.Duration(secs*float64(time.Second)), nil
}
