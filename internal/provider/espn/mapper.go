package espn

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

func gameID(league core.League, externalID string) core.GameID {
	return core.GameID(string(league) + ":" + externalID)
}

func externalID(id core.GameID) (string, error) {
	_, ext, ok := strings.Cut(string(id), ":")
	if !ok || ext == "" {
		return "", fmt.Errorf("game id %q has no provider part", id)
	}
	return ext, nil
}

func refID(ref string) string {
	if ref == "" {
		return ""
	}
	if i := strings.IndexByte(ref, '?'); i >= 0 {
		ref = ref[:i]
	}
	return path.Base(ref)
}

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date %q", s)
}

func mapStatus(name, state string, completed bool) core.GameStatus {
	switch name {
	case "STATUS_FINAL":
		return core.GameFinal
	case "STATUS_POSTPONED", "STATUS_CANCELED", "STATUS_SUSPENDED":
		return core.GamePostponed
	case "STATUS_SCHEDULED":
		return core.GameScheduled
	}

	switch {
	case completed:
		return core.GameFinal
	case state == "in":
		return core.GameLive
	default:
		return core.GameScheduled
	}
}

func (c *Client) mapScoreboard(league core.League, resp scoreboardResponse, observedAt time.Time) ([]core.Game, error) {
	var out []core.Game

	for _, e := range resp.Events {
		if len(e.Competitions) == 0 {
			continue
		}
		comp := e.Competitions[0]

		startsAt, err := parseDate(e.Date)
		if err != nil {
			return nil, fmt.Errorf("game %s: %w", e.ID, err)
		}

		game := core.Game{
			ID:         gameID(league, e.ID),
			League:     league,
			StartsAt:   startsAt,
			Status:     mapStatus(comp.Status.Type.Name, comp.Status.Type.State, comp.Status.Type.Completed),
			Period:     comp.Status.Period,
			Clock:      comp.Status.DisplayClock,
			ObservedAt: observedAt,
		}

		for _, side := range comp.Competitors {
			score, _ := strconv.Atoi(side.Score)
			ts := core.TeamScore{
				Team: core.Team{
					League: league,
					Code:   core.TeamCode(side.Team.Abbreviation),
					Name:   side.Team.DisplayName,
				},
				Score: score,
			}
			if side.HomeAway == "home" {
				game.Home = ts
			} else {
				game.Away = ts
			}
		}

		if game.Home.Team.IsZero() || game.Away.Team.IsZero() {
			continue
		}
		if game.IsFinal() {
			game.Clock = ""
		}
		out = append(out, game)
	}
	return out, nil
}

func mapTeams(league core.League, resp teamsResponse) []core.ExternalTeam {
	var out []core.ExternalTeam
	for _, sport := range resp.Sports {
		for _, l := range sport.Leagues {
			for _, entry := range l.Teams {
				t := entry.Team
				if t.Abbreviation == "" {
					continue
				}
				out = append(out, core.ExternalTeam{
					ExternalID: t.ID,
					Team: core.Team{
						League: league,
						Code:   core.TeamCode(t.Abbreviation),
						Name:   t.DisplayName,
					},
				})
			}
		}
	}
	return out
}

func mapPlays(resp playsResponse, codeOf map[string]core.TeamCode) []core.Play {
	var out []core.Play

	for _, item := range resp.Items {
		seq, err := strconv.Atoi(item.SequenceNumber)
		if err != nil {
			continue
		}

		play := core.Play{
			Seq:       seq,
			Period:    item.Period.Number,
			Clock:     item.Clock.DisplayValue,
			HomeScore: item.HomeScore,
			AwayScore: item.AwayScore,
			Team:      codeOf[refID(item.Team.Ref)],
		}
		if item.ScoringPlay {
			play.Points = item.ScoreValue
		}
		if len(item.Participants) > 0 {
			play.Player = core.Player{
				ID:   core.PlayerID(refID(item.Participants[0].Athlete.Ref)),
				Team: play.Team,
			}
		}
		if at, err := parseDate(item.Wallclock); err == nil {
			play.OccurredAt = at
		}
		out = append(out, play)
	}
	return out
}

func mapBoxScore(game core.Game, resp summaryResponse) core.GameStats {
	stats := core.GameStats{GameID: game.ID, League: game.League}

	for _, block := range resp.Boxscore.Players {
		if len(block.Statistics) == 0 {
			continue
		}
		st := block.Statistics[0]
		idx := statIndex(st.Names)

		for _, a := range st.Athletes {
			if len(a.Stats) == 0 {
				continue
			}
			stats.Lines = append(stats.Lines, core.PlayerLine{
				Player: core.Player{
					ID:   core.PlayerID(a.Athlete.ID),
					Name: a.Athlete.DisplayName,
					Team: core.TeamCode(block.Team.Abbreviation),
				},
				Points:   statValue(a.Stats, idx, "PTS"),
				Rebounds: statValue(a.Stats, idx, "REB"),
				Assists:  statValue(a.Stats, idx, "AST"),
			})
		}
	}
	return stats
}

func statIndex(names []string) map[string]int {
	idx := make(map[string]int, len(names))
	for i, n := range names {
		idx[n] = i
	}
	return idx
}

func statValue(stats []string, idx map[string]int, name string) int {
	i, ok := idx[name]
	if !ok || i < 0 || i >= len(stats) {
		return 0
	}
	v, err := strconv.Atoi(stats[i])
	if err != nil {
		return 0
	}
	return v
}
