package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/damnary/nba-digest/internal/core"
)

var periodNames = map[int]string{
	1: "1-я четверть",
	2: "2-я четверть",
	3: "3-я четверть",
	4: "4-я четверть",
}

func (c *Client) SendEvent(ctx context.Context, chatID int64, event core.Event) error {
	return c.SendMessage(ctx, chatID, FormatEvent(event), nil)
}

func (c *Client) SendDigest(ctx context.Context, chatID int64, digest core.Digest) error {
	return c.SendMessage(ctx, chatID, FormatDigest(digest), nil)
}

func FormatEvent(event core.Event) string {
	score := fmt.Sprintf("%s %d : %d %s", teamOf(event, 0), event.HomeScore, event.AwayScore, teamOf(event, 1))

	switch event.Kind {
	case core.EventGameStarted:
		return fmt.Sprintf("🏀 Начался матч %s — %s", teamOf(event, 0), teamOf(event, 1))

	case core.EventGameFinal:
		return fmt.Sprintf("🏁 Финал: %s", score)

	case core.EventRun:
		return fmt.Sprintf("🔥 %s: рывок %d:%d\n%s, %s",
			event.Run.Team, event.Run.Points, event.Run.Against, score, moment(event))

	case core.EventLeadChange:
		return fmt.Sprintf("🔄 %s отыгрались и вышли вперёд\n%s, %s",
			event.Run.Team, score, moment(event))

	case core.EventCloseFinish:
		return fmt.Sprintf("⏱ Концовка на волоске: %s\n%s", score, moment(event))
	}
	return score
}

func FormatDigest(digest core.Digest) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Матчи за %s\n", digest.Day)

	for _, game := range digest.FeaturedGames() {
		b.WriteString("\n")
		writeFeatured(&b, game)
	}

	rest := digest.RestGames()
	if len(rest) > 0 {
		b.WriteString("\nОстальные матчи:\n")
		for _, game := range rest {
			writeBrief(&b, game)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeFeatured(b *strings.Builder, game core.DigestGame) {
	g := game.Game
	fmt.Fprintf(b, "%s %d : %d %s\n", g.Home.Team.Name, g.Home.Score, g.Away.Score, g.Away.Team.Name)

	for _, side := range []core.TeamScore{g.Home, g.Away} {
		for _, line := range game.Stats.ByTeam(side.Team.Code).TopScorers(2) {
			fmt.Fprintf(b, "  %s — %d очк.\n", line.Player.Name, line.Points)
		}
	}

	if line, ok := game.Stats.TopRebounder(); ok {
		fmt.Fprintf(b, "  подборы: %s (%d)\n", line.Player.Name, line.Rebounds)
	}
	if line, ok := game.Stats.TopAssister(); ok {
		fmt.Fprintf(b, "  передачи: %s (%d)\n", line.Player.Name, line.Assists)
	}
	if line, ok := game.Stats.ClutchLeader(); ok {
		fmt.Fprintf(b, "  вытащил концовку: %s (%d очк. с 4-й)\n", line.Player.Name, line.ClutchPoints)
	}
}

func writeBrief(b *strings.Builder, game core.DigestGame) {
	g := game.Game
	fmt.Fprintf(b, "%s %d : %d %s", g.Home.Team.Code, g.Home.Score, g.Away.Score, g.Away.Team.Code)

	if top := game.Stats.TopScorers(1); len(top) > 0 {
		fmt.Fprintf(b, " — %s %d", top[0].Player.Name, top[0].Points)
	}
	b.WriteString("\n")
}

func moment(event core.Event) string {
	period, ok := periodNames[event.Period]
	if !ok {
		period = fmt.Sprintf("овертайм %d", event.Period-4)
	}
	if event.Clock == "" {
		return period
	}
	return period + ", " + event.Clock
}

func teamOf(event core.Event, i int) core.TeamCode {
	if i < len(event.Teams) {
		return event.Teams[i]
	}
	return ""
}
