package core

import (
	"fmt"
	"slices"
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

var notifiableKinds = []EventKind{EventCloseFinish}

func NotifiableKinds() []EventKind { return slices.Clone(notifiableKinds) }

func (k EventKind) Notifiable() bool { return slices.Contains(notifiableKinds, k) }

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
