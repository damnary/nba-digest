package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

type Provider struct {
	fixtures []fixture
	speed    float64
	started  time.Time
	now      func() time.Time
}

type Option func(*Provider)

func WithClock(now func() time.Time) Option {
	return func(p *Provider) { p.now = now }
}

func WithStart(t time.Time) Option {
	return func(p *Provider) { p.started = t }
}

func New(fsys fs.FS, speed float64, opts ...Option) (*Provider, error) {
	if speed <= 0 {
		return nil, fmt.Errorf("replay speed must be positive, got %v", speed)
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read replay dir: %w", err)
	}

	p := &Provider{speed: speed, now: time.Now}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var f fixture
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		if err := f.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		p.fixtures = append(p.fixtures, f)
	}

	if len(p.fixtures) == 0 {
		return nil, fmt.Errorf("no replay fixtures found")
	}

	for _, opt := range opts {
		opt(p)
	}
	if p.started.IsZero() {
		p.started = p.now()
	}
	return p, nil
}

func (p *Provider) Name() string { return "replay" }

func (p *Provider) Teams(_ context.Context, league core.League) ([]core.ExternalTeam, error) {
	seen := make(map[core.TeamCode]bool)
	var out []core.ExternalTeam

	for _, f := range p.fixtures {
		if core.League(f.League) != league {
			continue
		}
		for _, t := range []teamJSON{f.Home, f.Away} {
			code := core.TeamCode(t.Code)
			if seen[code] {
				continue
			}
			seen[code] = true
			out = append(out, core.ExternalTeam{
				ExternalID: t.Code,
				Team:       core.Team{League: league, Code: code, Name: t.Name},
			})
		}
	}
	return out, nil
}

func (p *Provider) Games(_ context.Context, league core.League, _ core.Day) ([]core.Game, error) {
	var out []core.Game
	for _, f := range p.fixtures {
		if core.League(f.League) != league {
			continue
		}
		out = append(out, p.gameState(f))
	}
	return out, nil
}

func (p *Provider) Plays(_ context.Context, game core.Game, _ string) (core.PlayFeed, error) {
	f, ok := p.fixtureOf(game.ID)
	if !ok {
		return core.PlayFeed{}, fmt.Errorf("unknown game %s", game.ID)
	}

	plays := p.visiblePlays(f)
	feed := core.PlayFeed{Plays: plays}
	if n := len(plays); n > 0 {
		feed.Cursor = fmt.Sprint(plays[n-1].Seq)
	}
	return feed, nil
}

func (p *Provider) BoxScore(_ context.Context, game core.Game) (core.GameStats, error) {
	f, ok := p.fixtureOf(game.ID)
	if !ok {
		return core.GameStats{}, fmt.Errorf("unknown game %s", game.ID)
	}

	stats := core.GameStats{GameID: game.ID, League: core.League(f.League)}
	for _, l := range f.BoxScore {
		stats.Lines = append(stats.Lines, core.PlayerLine{
			Player: core.Player{
				ID:   core.PlayerID(l.PlayerID),
				Name: l.Name,
				Team: core.TeamCode(l.Team),
			},
			Points:   l.Points,
			Rebounds: l.Rebounds,
			Assists:  l.Assists,
		})
	}
	return stats, nil
}

func (p *Provider) elapsed() time.Duration {
	return time.Duration(float64(p.now().Sub(p.started)) * p.speed)
}

func (p *Provider) wallTime(offsetSeconds int) time.Time {
	return p.started.Add(time.Duration(float64(offsetSeconds) * float64(time.Second) / p.speed))
}

func (p *Provider) fixtureOf(id core.GameID) (fixture, bool) {
	for _, f := range p.fixtures {
		if core.GameID(f.GameID) == id {
			return f, true
		}
	}
	return fixture{}, false
}

func (p *Provider) visiblePlays(f fixture) []core.Play {
	elapsed := p.elapsed()
	if elapsed < 0 {
		return nil
	}

	var out []core.Play
	for _, pl := range f.Plays {
		if time.Duration(pl.OffsetSeconds)*time.Second > elapsed {
			break
		}
		out = append(out, core.Play{
			Seq:        pl.Seq,
			Period:     pl.Period,
			Clock:      pl.Clock,
			Team:       core.TeamCode(pl.Team),
			Points:     pl.Points,
			Player:     core.Player{ID: core.PlayerID(pl.PlayerID), Name: pl.PlayerName, Team: core.TeamCode(pl.Team)},
			HomeScore:  pl.HomeScore,
			AwayScore:  pl.AwayScore,
			OccurredAt: p.wallTime(pl.OffsetSeconds),
		})
	}
	return out
}

func (p *Provider) gameState(f fixture) core.Game {
	league := core.League(f.League)
	game := core.Game{
		ID:       core.GameID(f.GameID),
		League:   league,
		StartsAt: p.started,
		Status:   core.GameScheduled,
		Home:     core.TeamScore{Team: core.Team{League: league, Code: core.TeamCode(f.Home.Code), Name: f.Home.Name}},
		Away:     core.TeamScore{Team: core.Team{League: league, Code: core.TeamCode(f.Away.Code), Name: f.Away.Name}},
	}

	visible := p.visiblePlays(f)
	if len(visible) == 0 {
		game.ObservedAt = p.now()
		return game
	}

	last := visible[len(visible)-1]
	game.Status = core.GameLive
	game.Period = last.Period
	game.Clock = last.Clock
	game.Home.Score = last.HomeScore
	game.Away.Score = last.AwayScore
	game.ObservedAt = p.now()

	if len(visible) == len(f.Plays) {
		game.Status = core.GameFinal
		game.Clock = ""
	}
	return game
}
