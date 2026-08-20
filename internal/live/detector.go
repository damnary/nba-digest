package live

import (
	"slices"

	"github.com/damnary/nba-digest/internal/core"
)

func Detect(game core.Game, plays []core.Play) []core.Event {
	var events []core.Event

	if game.Status == core.GameLive || game.IsFinal() {
		events = append(events, lifecycleEvent(game, core.EventGameStarted))
	}
	if game.IsFinal() {
		events = append(events, lifecycleEvent(game, core.EventGameFinal))
	}

	events = append(events, detectRuns(game, plays)...)
	events = append(events, detectComebacks(game, plays)...)
	events = append(events, detectCloseFinish(game, plays)...)

	slices.SortStableFunc(events, func(a, b core.Event) int {
		return a.OccurredAt.Compare(b.OccurredAt)
	})
	return events
}

type run struct {
	team    core.TeamCode
	scored  int
	allowed int
	emitted bool
}

func detectRuns(game core.Game, plays []core.Play) []core.Event {
	var (
		out     []core.Event
		current run
	)

	for _, p := range plays {
		if p.Points <= 0 || p.Team == "" {
			continue
		}

		switch {
		case current.team == "":
			current = run{team: p.Team, scored: p.Points}
		case p.Team == current.team:
			current.scored += p.Points
		default:
			current.allowed += p.Points
			if current.allowed > core.RunTolerance {
				current = run{team: p.Team, scored: current.allowed}
			}
		}

		if current.emitted || current.scored < core.RunMinPoints {
			continue
		}
		current.emitted = true

		ev := eventFromPlay(game, p, core.EventRun)
		ev.Run = core.RunInfo{Team: current.team, Points: current.scored, Against: current.allowed}
		out = append(out, ev)
	}
	return out
}

func detectComebacks(game core.Game, plays []core.Play) []core.Event {
	var (
		out                []core.Event
		homeDeficit        int
		awayDeficit        int
		previous           int
		homeCode, awayCode = game.Home.Team.Code, game.Away.Team.Code
	)

	for _, p := range plays {
		margin := p.HomeScore - p.AwayScore

		switch {
		case margin < 0 && -margin > homeDeficit:
			homeDeficit = -margin
		case margin > 0 && margin > awayDeficit:
			awayDeficit = margin
		}

		if margin > 0 && previous <= 0 {
			if homeDeficit >= core.ComebackDeficit {
				out = append(out, comebackEvent(game, p, homeCode))
			}
			homeDeficit = 0
		}
		if margin < 0 && previous >= 0 {
			if awayDeficit >= core.ComebackDeficit {
				out = append(out, comebackEvent(game, p, awayCode))
			}
			awayDeficit = 0
		}
		previous = margin
	}
	return out
}

func detectCloseFinish(game core.Game, plays []core.Play) []core.Event {
	for _, p := range plays {
		if !p.IsClutch() || p.Margin() > core.CloseFinishMargin {
			continue
		}
		remaining, err := core.ParseClock(p.Clock)
		if err != nil || remaining > core.CloseFinishWindow {
			continue
		}
		return []core.Event{eventFromPlay(game, p, core.EventCloseFinish)}
	}
	return nil
}

func comebackEvent(game core.Game, p core.Play, team core.TeamCode) core.Event {
	ev := eventFromPlay(game, p, core.EventLeadChange)
	ev.Run = core.RunInfo{Team: team}
	return ev
}

func eventFromPlay(game core.Game, p core.Play, kind core.EventKind) core.Event {
	return core.Event{
		ID:         core.NewEventID(game.ID, kind, p.Period, p.Seq),
		GameID:     game.ID,
		League:     game.League,
		Kind:       kind,
		Teams:      []core.TeamCode{game.Home.Team.Code, game.Away.Team.Code},
		Period:     p.Period,
		Clock:      p.Clock,
		HomeScore:  p.HomeScore,
		AwayScore:  p.AwayScore,
		OccurredAt: p.OccurredAt,
	}
}

func lifecycleEvent(game core.Game, kind core.EventKind) core.Event {
	occurredAt := game.StartsAt
	if kind == core.EventGameFinal {
		occurredAt = game.ObservedAt
	}

	return core.Event{
		ID:         core.NewEventID(game.ID, kind, 0, 0),
		GameID:     game.ID,
		League:     game.League,
		Kind:       kind,
		Teams:      []core.TeamCode{game.Home.Team.Code, game.Away.Team.Code},
		Period:     game.Period,
		HomeScore:  game.Home.Score,
		AwayScore:  game.Away.Score,
		OccurredAt: occurredAt,
	}
}
