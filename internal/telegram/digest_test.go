package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

func team(code, name string) core.Team {
	return core.Team{League: core.LeagueNBA, Code: core.TeamCode(code), Name: name}
}

func digestGame(home, homeName string, hs int, away, awayName string, as int, featured bool) core.DigestGame {
	id := core.GameID("nba:" + home + away)
	return core.DigestGame{
		Game: core.Game{
			ID:     id,
			League: core.LeagueNBA,
			Status: core.GameFinal,
			Home:   core.TeamScore{Team: team(home, homeName), Score: hs},
			Away:   core.TeamScore{Team: team(away, awayName), Score: as},
		},
		Stats:    core.GameStats{GameID: id, League: core.LeagueNBA},
		Featured: featured,
	}
}

func testDigest(games ...core.DigestGame) core.Digest {
	return core.Digest{
		Subscriber: 1,
		League:     core.LeagueNBA,
		Day:        core.Day{Year: 2026, Month: time.October, Day: 28},
		Games:      games,
	}
}

func TestWinnerComesFirstWhoeverPlayedAtHome(t *testing.T) {
	homeWon := testDigest(digestGame("NY", "New York Knicks", 133, "CHI", "Chicago Bulls", 127, true))
	awayWon := testDigest(digestGame("CHI", "Chicago Bulls", 127, "NY", "New York Knicks", 133, true))

	for name, text := range map[string]string{"home won": FormatDigest(homeWon), "away won": FormatDigest(awayWon)} {
		if !strings.Contains(text, "<b>New York Knicks 133</b> — 127 Chicago Bulls") {
			t.Errorf("%s: the winner must open the line, got:\n%s", name, text)
		}
	}
}

func TestEveryPartHasBalancedBlockquotes(t *testing.T) {
	var games []core.DigestGame
	for i := range 30 {
		g := digestGame("H"+string(rune('A'+i)), "Home Team", 100+i, "A"+string(rune('A'+i)), "Away Team", 90+i, i < 24)
		g.Stats = core.GameStats{
			GameID: g.Game.ID,
			League: core.LeagueNBA,
			Lines: []core.PlayerLine{
				{Player: core.Player{ID: "p1", Name: "Giannis Antetokounmpo", Team: g.Game.Home.Team.Code}, Points: 31, Rebounds: 12, Assists: 4, ClutchPoints: 9},
				{Player: core.Player{ID: "p2", Name: "Shai Gilgeous-Alexander", Team: g.Game.Home.Team.Code}, Points: 24},
				{Player: core.Player{ID: "p3", Name: "Victor Wembanyama", Team: g.Game.Away.Team.Code}, Points: 28, Assists: 11},
				{Player: core.Player{ID: "p4", Name: "Jalen Williams", Team: g.Game.Away.Team.Code}, Points: 19},
			},
		}
		games = append(games, g)
	}

	parts := DigestParts(testDigest(games...))
	if len(parts) < 2 {
		t.Fatalf("a 30-game digest must span several messages, got %d", len(parts))
	}
	for i, part := range parts {
		if len(part) > maxMessageBytes {
			t.Errorf("part %d is %d bytes, over the limit", i, len(part))
		}
		if open, closed := strings.Count(part, "<blockquote"), strings.Count(part, "</blockquote>"); open != closed {
			t.Errorf("part %d splits a quote open: %d opened, %d closed", i, open, closed)
		}
		if open, closed := strings.Count(part, "<b>"), strings.Count(part, "</b>"); open != closed {
			t.Errorf("part %d has unbalanced bold tags", i)
		}
		if open, closed := strings.Count(part, "<code>"), strings.Count(part, "</code>"); open != closed {
			t.Errorf("part %d has unbalanced code tags", i)
		}
	}
}

func TestRestIsCollapsedOnlyWhenSomethingElseIsShown(t *testing.T) {
	withFeatured := FormatDigest(testDigest(
		digestGame("NY", "New York Knicks", 133, "CHI", "Chicago Bulls", 127, true),
		digestGame("LAL", "Los Angeles Lakers", 103, "LAC", "LA Clippers", 87, false),
	))
	if !strings.Contains(withFeatured, "<blockquote expandable>") {
		t.Error("other games should be collapsed when the subscriber has games of their own")
	}

	onlyRest := FormatDigest(testDigest(digestGame("LAL", "Los Angeles Lakers", 103, "LAC", "LA Clippers", 87, false)))
	if strings.Contains(onlyRest, "expandable") {
		t.Error("a digest that is nothing but other games must not hide all of itself behind a tap")
	}
}

func TestGameWithoutABoxScoreStillRenders(t *testing.T) {
	text := FormatDigest(testDigest(digestGame("NY", "New York Knicks", 133, "CHI", "Chicago Bulls", 127, true)))

	if !strings.Contains(text, "New York Knicks 133") {
		t.Fatalf("score is missing:\n%s", text)
	}
	if strings.Contains(text, "подборы") || strings.Contains(text, "концовку") {
		t.Errorf("no stats means no stat lines:\n%s", text)
	}
	if strings.Contains(text, "\n\n\n") {
		t.Errorf("missing stats left a hole in the block:\n%s", text)
	}
}

func TestCountsUseRussianPlurals(t *testing.T) {
	cases := map[int]string{1: "1 матч", 2: "2 матча", 5: "5 матчей", 11: "11 матчей", 21: "21 матч", 102: "102 матча"}
	for n, want := range cases {
		if got := plural(n, "матч", "матча", "матчей"); got != want {
			t.Errorf("plural(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestLastNameKeepsSuffixesAndSingleNames(t *testing.T) {
	cases := map[string]string{
		"Jalen Brunson":         "Brunson",
		"Derrick Jones Jr.":     "Jones Jr.",
		"Giannis Antetokounmpo": "Antetokounmpo",
		"Nikola Jokic":          "Jokic",
		"Bol":                   "Bol",
		"Karl-Anthony Towns":    "Towns",
	}
	for full, want := range cases {
		if got := lastName(full); got != want {
			t.Errorf("lastName(%q) = %q, want %q", full, got, want)
		}
	}
}

func TestScoreColumnsLineUp(t *testing.T) {
	text := FormatDigest(testDigest(
		digestGame("NY", "New York Knicks", 133, "CHI", "Chicago Bulls", 127, true),
		digestGame("UTAH", "Utah Jazz", 111, "SA", "San Antonio Spurs", 117, false),
		digestGame("DET", "Detroit Pistons", 97, "CHA", "Charlotte Hornets", 89, false),
	))

	var colons []int
	for _, line := range strings.Split(text, "\n") {
		start := strings.Index(line, "<code>")
		if start < 0 {
			continue
		}
		row := line[start+len("<code>") : strings.Index(line, "</code>")]
		colons = append(colons, strings.Index(row, ":"))
	}
	if len(colons) != 2 {
		t.Fatalf("want 2 compact rows, got %d", len(colons))
	}
	if colons[0] != colons[1] {
		t.Errorf("scores must share a column whatever the code lengths, colon at %v", colons)
	}
}

func scoredGame(featured bool) core.DigestGame {
	g := digestGame("NY", "New York Knicks", 133, "CHI", "Chicago Bulls", 127, featured)
	g.Stats = core.GameStats{
		GameID:       g.Game.ID,
		League:       core.LeagueNBA,
		ClutchMargin: 6,
		Lines: []core.PlayerLine{
			{Player: core.Player{ID: "w1", Name: "Jalen Brunson", Team: "NY"}, Points: 38, Rebounds: 3, Assists: 9, ClutchPoints: 11},
			{Player: core.Player{ID: "w2", Name: "Karl-Anthony Towns", Team: "NY"}, Points: 21, Rebounds: 13, Assists: 2, ClutchPoints: 4},
			{Player: core.Player{ID: "l1", Name: "Norman Powell", Team: "CHI"}, Points: 30, Rebounds: 4, Assists: 1, ClutchPoints: 17},
			{Player: core.Player{ID: "l2", Name: "Josh Giddey", Team: "CHI"}, Points: 14, Rebounds: 8, Assists: 12, ClutchPoints: 2},
		},
	}
	return g
}

func TestClutchLineCreditsTheWinner(t *testing.T) {
	text := FormatDigest(testDigest(scoredGame(true)))

	if !strings.Contains(text, "концовку вытащил Brunson") {
		t.Errorf("the clutch line must name a winner, got:\n%s", text)
	}
	if strings.Contains(text, "концовку вытащил Powell") {
		t.Error("Powell scored most in the fourth but his team lost: nobody 'pulls out' a defeat")
	}
}

func TestClutchLineStaysHiddenInABlowout(t *testing.T) {
	game := scoredGame(true)
	game.Stats.ClutchMargin = 24
	game.Game.Home.Score = 133
	game.Game.Away.Score = 99

	if text := FormatDigest(testDigest(game)); strings.Contains(text, "концовку") {
		t.Errorf("a 34-point win has no clutch finish to report:\n%s", text)
	}
}

func TestEachTeamGetsItsOwnLeaders(t *testing.T) {
	text := FormatDigest(testDigest(scoredGame(true)))

	wantLines := []string{
		"NY — Brunson 38, Towns 21",
		"подборы Towns 13 · передачи Brunson 9",
		"CHI — Powell 30, Giddey 14",
		"подборы Giddey 8 · передачи Giddey 12",
	}
	for _, want := range wantLines {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}

	lines := strings.Split(text, "\n")
	nyAt, chiAt := -1, -1
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "NY — "):
			nyAt = i
		case strings.HasPrefix(line, "CHI — "):
			chiAt = i
		}
	}
	if nyAt < 0 || chiAt < 0 || chiAt <= nyAt {
		t.Fatalf("teams out of order: NY at %d, CHI at %d", nyAt, chiAt)
	}
	if lines[nyAt+1] != "подборы Towns 13 · передачи Brunson 9" {
		t.Errorf("a team's leaders must sit right under its own scorers, got %q", lines[nyAt+1])
	}
}
