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

var monthsRU = [...]string{
	"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func esc(s string) string { return htmlEscaper.Replace(s) }

func (c *Client) SendEvent(ctx context.Context, chatID int64, event core.Event) error {
	return c.SendMessage(ctx, chatID, FormatEvent(event), nil)
}

func (c *Client) SendDigest(ctx context.Context, chatID int64, digest core.Digest) error {
	return c.SendMessage(ctx, chatID, FormatDigest(digest), nil)
}

func FormatEvent(event core.Event) string {
	score := fmt.Sprintf("%s  %d : %d  %s",
		teamOf(event, 0), event.HomeScore, event.AwayScore, teamOf(event, 1))

	switch event.Kind {
	case core.EventGameStarted:
		return fmt.Sprintf("<b>Матч начался</b>\n\n<b>%s — %s</b>", teamOf(event, 0), teamOf(event, 1))

	case core.EventGameFinal:
		return fmt.Sprintf("<b>Финал</b>\n\n<b>%s</b>", score)

	case core.EventRun:
		return fmt.Sprintf("<b>Рывок %d:%d — %s</b>\n\n<b>%s</b>\n%s",
			event.Run.Points, event.Run.Against, event.Run.Team, score, moment(event))

	case core.EventLeadChange:
		return fmt.Sprintf("<b>Камбэк — %s впереди</b>\n\n<b>%s</b>\n%s",
			event.Run.Team, score, moment(event))

	case core.EventCloseFinish:
		return fmt.Sprintf("<b>Концовка на волоске</b>\n\n<b>%s</b>\n%s", score, moment(event))
	}
	return score
}

func FormatDigest(digest core.Digest) string {
	var b strings.Builder

	fmt.Fprintf(&b, "<b>Матчи за %s</b>\n", formatDay(digest.Day))

	for _, game := range digest.FeaturedGames() {
		b.WriteString("\n")
		writeFeatured(&b, game)
	}

	rest := digest.RestGames()
	if len(rest) > 0 {
		b.WriteString("\n———\n\n<b>Остальные матчи</b>\n\n")
		for _, game := range rest {
			writeBrief(&b, game)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeFeatured(b *strings.Builder, game core.DigestGame) {
	g := game.Game
	fmt.Fprintf(b, "<b>%s  %d : %d  %s</b>\n\n",
		esc(g.Home.Team.Name), g.Home.Score, g.Away.Score, esc(g.Away.Team.Name))

	for _, side := range []core.TeamScore{g.Home, g.Away} {
		top := game.Stats.ByTeam(side.Team.Code).TopScorers(2)
		if len(top) == 0 {
			continue
		}

		scorers := make([]string, len(top))
		for i, l := range top {
			scorers[i] = fmt.Sprintf("%s %d", esc(l.Player.Name), l.Points)
		}
		fmt.Fprintf(b, "%s — %s\n", side.Team.Code, strings.Join(scorers, ", "))
	}

	var extras []string
	if l, ok := game.Stats.TopRebounder(); ok {
		extras = append(extras, fmt.Sprintf("Подборы: %s (%d)", esc(l.Player.Name), l.Rebounds))
	}
	if l, ok := game.Stats.TopAssister(); ok {
		extras = append(extras, fmt.Sprintf("Передачи: %s (%d)", esc(l.Player.Name), l.Assists))
	}
	if l, ok := game.Stats.ClutchLeader(); ok {
		extras = append(extras, fmt.Sprintf("Концовку вытащил: %s (%d с 4-й)", esc(l.Player.Name), l.ClutchPoints))
	}
	if len(extras) > 0 {
		b.WriteString("\n" + strings.Join(extras, "\n") + "\n")
	}
}

func writeBrief(b *strings.Builder, game core.DigestGame) {
	g := game.Game
	fmt.Fprintf(b, "%s  <b>%d : %d</b>  %s", g.Home.Team.Code, g.Home.Score, g.Away.Score, g.Away.Team.Code)

	if top := game.Stats.TopScorers(1); len(top) > 0 {
		fmt.Fprintf(b, " — %s %d", esc(top[0].Player.Name), top[0].Points)
	}
	b.WriteString("\n")
}

func formatDay(d core.Day) string {
	month := int(d.Month) - 1
	if month < 0 || month >= len(monthsRU) {
		return d.String()
	}
	return fmt.Sprintf("%d %s", d.Day, monthsRU[month])
}

func moment(event core.Event) string {
	period, ok := periodNames[event.Period]
	if !ok {
		period = fmt.Sprintf("овертайм %d", event.Period-4)
	}
	if event.Clock == "" {
		return period
	}
	return period + " · " + event.Clock
}

func teamOf(event core.Event, i int) core.TeamCode {
	if i < len(event.Teams) {
		return event.Teams[i]
	}
	return ""
}
