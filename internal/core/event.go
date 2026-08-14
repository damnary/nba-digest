package core

import (
	"fmt"
	"time"
)

type EventID string

type EventKind string

const (
	EventGameStarted EventKind = "game_started"
	EventGameFinal   EventKind = "game_final"
	EventRun         EventKind = "run"
	EventLeadChange  EventKind = "lead_change"
	EventCloseFinish EventKind = "close_finish"
)

type RunInfo struct {
	Team    TeamCode
	Points  int
	Against int
}

type Event struct {
	ID     EventID
	GameID GameID
	League League
	Kind   EventKind

	Teams []TeamCode

	Period     int
	Clock      string
	HomeScore  int
	AwayScore  int
	Run        RunInfo
	OccurredAt time.Time
}

func NewEventID(game GameID, kind EventKind, period, seq int) EventID {
	return EventID(fmt.Sprintf("%s:%s:p%d:%06d", game, kind, period, seq))
}

func (e Event) Concerns(code TeamCode) bool {
	for _, t := range e.Teams {
		if t == code {
			return true
		}
	}
	return false
}
