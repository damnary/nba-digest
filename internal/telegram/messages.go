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
	parts := DigestParts(digest)
	for i, part := range parts {
		var kb *keyboard
		if i == len(parts)-1 {
			kb = digestFooter()
		}
		if err := c.SendMessage(ctx, chatID, part, kb); err != nil {
			return err
		}
	}
	return nil
}

func digestFooter() *keyboard {
	return &keyboard{Rows: [][]button{{
		{Text: "Таблица", Data: standingsData},
		{Text: "Меню", Data: menuData},
	}}}
}

const maxMessageBytes = 4000

func splitMessage(text string) []string {
	if len(text) <= maxMessageBytes {
		return []string{text}
	}

	var (
		parts []string
		b     strings.Builder
	)
	flush := func() {
		if b.Len() > 0 {
			parts = append(parts, b.String())
			b.Reset()
		}
	}

	for _, paragraph := range strings.Split(text, "\n\n") {
		if b.Len() > 0 && b.Len()+2+len(paragraph) > maxMessageBytes {
			flush()
		}
		for len(paragraph) > maxMessageBytes {
			flush()
			parts = append(parts, paragraph[:maxMessageBytes])
			paragraph = paragraph[maxMessageBytes:]
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(paragraph)
	}
	flush()
	return parts
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
