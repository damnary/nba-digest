package live

import (
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

var baseTime = time.Date(2026, 7, 27, 23, 0, 0, 0, time.UTC)

func testGame(status core.GameStatus) core.Game {
	return core.Game{
		ID:       "wnba:1",
		League:   core.LeagueWNBA,
		StartsAt: baseTime,
		Status:   status,
		Home:     core.TeamScore{Team: core.Team{League: core.LeagueWNBA, Code: "NYL"}},
		Away:     core.TeamScore{Team: core.Team{League: core.LeagueWNBA, Code: "LVA"}},
	}
}

type scoring struct {
	team   core.TeamCode
	points int
}

// buildPlays turns a scoring sequence into a play-by-play feed, keeping the
// running score consistent.
func buildPlays(period int, clock string, seq []scoring) []core.Play {
	var (
		plays      []core.Play
		home, away int
	)
	for i, s := range seq {
		if s.team == "NYL" {
			home += s.points
		} else {
			away += s.points
		}
		plays = append(plays, core.Play{
			Seq:        i + 1,
			Period:     period,
			Clock:      clock,
			Team:       s.team,
			Points:     s.points,
			HomeScore:  home,
			AwayScore:  away,
			OccurredAt: baseTime.Add(time.Duration(i) * time.Minute),
		})
	}
	return plays
}

func kinds(events []core.Event) []core.EventKind {
	out := make([]core.EventKind, len(events))
	for i, e := range events {
		out[i] = e.Kind
	}
	return out
}

func countKind(events []core.Event, kind core.EventKind) int {
	n := 0
	for _, e := range events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

func TestDetectRuns(t *testing.T) {
	tests := []struct {
		name      string
		seq       []scoring
		wantRuns  int
		wantTeam  core.TeamCode
		wantScore int
	}{
		{
			name: "clean 12-0 run is detected",
			seq: []scoring{
				{"NYL", 3}, {"NYL", 2}, {"NYL", 3}, {"NYL", 2}, {"NYL", 2},
			},
			wantRuns:  1,
			wantTeam:  "NYL",
			wantScore: 12,
		},
		{
			name: "run survives two points from the opponent",
			seq: []scoring{
				{"NYL", 3}, {"NYL", 3}, {"LVA", 2}, {"NYL", 3}, {"NYL", 3},
			},
			wantRuns:  1,
			wantTeam:  "NYL",
			wantScore: 12,
		},
		{
			name: "three points from the opponent break the run",
			seq: []scoring{
				{"NYL", 3}, {"NYL", 3}, {"LVA", 3}, {"NYL", 3}, {"NYL", 3},
			},
			wantRuns: 0,
		},
		{
			name: "eleven points are not enough",
			seq: []scoring{
				{"NYL", 3}, {"NYL", 3}, {"NYL", 3}, {"NYL", 2},
			},
			wantRuns: 0,
		},
		{
			name: "a long run is reported once, not on every basket",
			seq: []scoring{
				{"NYL", 3}, {"NYL", 3}, {"NYL", 3}, {"NYL", 3},
				{"NYL", 3}, {"NYL", 3}, {"NYL", 3},
			},
			wantRuns:  1,
			wantTeam:  "NYL",
			wantScore: 12,
		},
		{
			name: "both teams can have their own run",
			seq: []scoring{
				{"NYL", 3}, {"NYL", 3}, {"NYL", 3}, {"NYL", 3},
				{"LVA", 3}, {"LVA", 3}, {"LVA", 3}, {"LVA", 3},
			},
			wantRuns: 2,
		},
		{
			name:     "empty feed produces nothing",
			seq:      nil,
			wantRuns: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plays := buildPlays(2, "05:00", tt.seq)
			events := Detect(testGame(core.GameLive), plays)

			runs := countKind(events, core.EventRun)
			if runs != tt.wantRuns {
				t.Fatalf("got %d runs, want %d (all events: %v)", runs, tt.wantRuns, kinds(events))
			}
			if tt.wantRuns != 1 {
				return
			}
			for _, e := range events {
				if e.Kind != core.EventRun {
					continue
				}
				if e.Run.Team != tt.wantTeam {
					t.Errorf("run team = %s, want %s", e.Run.Team, tt.wantTeam)
				}
				if e.Run.Points != tt.wantScore {
					t.Errorf("run points = %d, want %d", e.Run.Points, tt.wantScore)
				}
			}
		})
	}
}

func TestDetectComebacks(t *testing.T) {
	tests := []struct {
		name          string
		seq           []scoring
		wantComebacks int
	}{
		{
			name: "erasing a ten point deficit is a comeback",
			seq: []scoring{
				{"LVA", 3}, {"LVA", 3}, {"LVA", 2}, {"LVA", 2},
				{"NYL", 3}, {"NYL", 3}, {"NYL", 3}, {"NYL", 3},
			},
			wantComebacks: 1,
		},
		{
			name: "erasing a six point deficit is not",
			seq: []scoring{
				{"LVA", 3}, {"LVA", 3},
				{"NYL", 3}, {"NYL", 3}, {"NYL", 2},
			},
			wantComebacks: 0,
		},
		{
			name: "trading the lead repeatedly is not a comeback",
			seq: []scoring{
				{"NYL", 2}, {"LVA", 3}, {"NYL", 2}, {"LVA", 3},
				{"NYL", 2}, {"LVA", 3}, {"NYL", 2},
			},
			wantComebacks: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plays := buildPlays(3, "05:00", tt.seq)
			events := Detect(testGame(core.GameLive), plays)

			got := countKind(events, core.EventLeadChange)
			if got != tt.wantComebacks {
				t.Errorf("got %d comebacks, want %d (all events: %v)", got, tt.wantComebacks, kinds(events))
			}
		})
	}
}

func TestDetectCloseFinish(t *testing.T) {
	tests := []struct {
		name   string
		period int
		clock  string
		home   int
		away   int
		want   int
	}{
		{name: "tight fourth quarter finish", period: 4, clock: "02:30", home: 88, away: 85, want: 1},
		{name: "overtime counts as clutch", period: 5, clock: "01:10", home: 99, away: 97, want: 1},
		{name: "too early in the quarter", period: 4, clock: "07:40", home: 88, away: 85, want: 0},
		{name: "margin too wide", period: 4, clock: "01:00", home: 95, away: 85, want: 0},
		{name: "third quarter is never clutch", period: 3, clock: "00:30", home: 70, away: 68, want: 0},
		{name: "seconds-only clock is parsed", period: 4, clock: "9.4", home: 88, away: 87, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plays := []core.Play{{
				Seq: 1, Period: tt.period, Clock: tt.clock,
				HomeScore: tt.home, AwayScore: tt.away, OccurredAt: baseTime,
			}}
			events := Detect(testGame(core.GameLive), plays)

			if got := countKind(events, core.EventCloseFinish); got != tt.want {
				t.Errorf("got %d close finishes, want %d", got, tt.want)
			}
		})
	}
}

func TestCloseFinishReportedOnce(t *testing.T) {
	var plays []core.Play
	for i := range 5 {
		plays = append(plays, core.Play{
			Seq: i + 1, Period: 4, Clock: "01:30",
			HomeScore: 88 + i, AwayScore: 86, OccurredAt: baseTime.Add(time.Duration(i) * time.Second),
		})
	}

	events := Detect(testGame(core.GameLive), plays)
	if got := countKind(events, core.EventCloseFinish); got != 1 {
		t.Errorf("got %d close finish events, want exactly 1", got)
	}
}

func TestLifecycleEvents(t *testing.T) {
	scheduled := Detect(testGame(core.GameScheduled), nil)
	if len(scheduled) != 0 {
		t.Errorf("scheduled game produced %v", kinds(scheduled))
	}

	live := Detect(testGame(core.GameLive), nil)
	if len(live) != 1 || live[0].Kind != core.EventGameStarted {
		t.Errorf("live game produced %v, want one game_started", kinds(live))
	}

	final := Detect(testGame(core.GameFinal), nil)
	if countKind(final, core.EventGameFinal) != 1 {
		t.Errorf("final game produced %v, want a game_final", kinds(final))
	}
}

func TestDetectIsDeterministic(t *testing.T) {
	plays := buildPlays(4, "02:00", []scoring{
		{"LVA", 3}, {"LVA", 3}, {"LVA", 3}, {"LVA", 3},
		{"NYL", 3}, {"NYL", 3}, {"NYL", 3}, {"NYL", 3}, {"NYL", 3},
	})
	game := testGame(core.GameFinal)

	first := Detect(game, plays)
	second := Detect(game, plays)

	if len(first) != len(second) {
		t.Fatalf("event count differs between runs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("event %d id differs: %q vs %q", i, first[i].ID, second[i].ID)
		}
	}

	// Re-running on a feed that grew must keep the ids of already known events.
	extended := append(slicesClone(plays), core.Play{
		Seq: len(plays) + 1, Period: 4, Clock: "00:20",
		Team: "LVA", Points: 2, HomeScore: 15, AwayScore: 14, OccurredAt: baseTime.Add(time.Hour),
	})
	grown := Detect(game, extended)

	known := make(map[core.EventID]bool)
	for _, e := range first {
		known[e.ID] = true
	}
	seen := 0
	for _, e := range grown {
		if known[e.ID] {
			seen++
		}
	}
	if seen != len(first) {
		t.Errorf("only %d of %d original events kept their id after the feed grew", seen, len(first))
	}
}

func slicesClone(in []core.Play) []core.Play {
	out := make([]core.Play, len(in))
	copy(out, in)
	return out
}
