package telegram

import (
	"fmt"
	"strings"

	"github.com/damnary/nba-digest/internal/core"
)

func FormatDigest(digest core.Digest) string {
	return strings.Join(DigestParts(digest), "\n\n")
}

func DigestParts(digest core.Digest) []string {
	blocks := digestBlocks(digest)
	if len(blocks) == 0 {
		return nil
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

	for _, block := range blocks {
		if b.Len() > 0 && b.Len()+2+len(block) > maxMessageBytes {
			flush()
		}
		if len(block) > maxMessageBytes {
			flush()
			parts = append(parts, splitMessage(block)...)
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(block)
	}
	flush()
	return parts
}

func digestBlocks(digest core.Digest) []string {
	featured := digest.FeaturedGames()
	rest := digest.RestGames()

	blocks := []string{heading(digest, len(featured), len(rest))}

	for _, game := range featured {
		blocks = append(blocks, featuredBlock(game))
	}
	if len(rest) > 0 {
		blocks = append(blocks, restBlock(rest, len(featured) > 0))
	}
	return blocks
}

func heading(digest core.Digest, featured, rest int) string {
	total := featured + rest
	line := fmt.Sprintf("<b>Матчи %s</b>", formatDay(digest.Day))
	if featured > 0 && rest > 0 {
		return line + fmt.Sprintf("\n%s, из них %d твоих", plural(total, "матч", "матча", "матчей"), featured)
	}
	return line + "\n" + plural(total, "матч", "матча", "матчей")
}

func featuredBlock(game core.DigestGame) string {
	var b strings.Builder
	winner, loser := sides(game.Game)

	b.WriteString("<blockquote>")
	fmt.Fprintf(&b, "<b>%s %d</b> — %d %s",
		esc(winner.Team.Name), winner.Score, loser.Score, esc(loser.Team.Name))

	for _, side := range []core.TeamScore{winner, loser} {
		writeSide(&b, game.Stats.ByTeam(side.Team.Code), side.Team.Code)
	}

	if l, ok := game.Stats.ByTeam(winner.Team.Code).ClutchLeader(); ok {
		fmt.Fprintf(&b, "\n\nконцовку вытащил %s — %s в четвёртой",
			esc(lastName(l.Player.Name)), plural(l.ClutchPoints, "очко", "очка", "очков"))
	}

	b.WriteString("</blockquote>")
	return b.String()
}

func writeSide(b *strings.Builder, stats core.GameStats, code core.TeamCode) {
	top := stats.TopScorers(2)
	if len(top) == 0 {
		return
	}

	scorers := make([]string, len(top))
	for i, l := range top {
		scorers[i] = fmt.Sprintf("%s %d", esc(lastName(l.Player.Name)), l.Points)
	}
	fmt.Fprintf(b, "\n\n%s — %s", esc(string(code)), strings.Join(scorers, ", "))

	var extras []string
	if l, ok := stats.TopRebounder(); ok {
		extras = append(extras, fmt.Sprintf("подборы %s %d", esc(lastName(l.Player.Name)), l.Rebounds))
	}
	if l, ok := stats.TopAssister(); ok {
		extras = append(extras, fmt.Sprintf("передачи %s %d", esc(lastName(l.Player.Name)), l.Assists))
	}
	if len(extras) > 0 {
		fmt.Fprintf(b, "\n%s", strings.Join(extras, " · "))
	}
}

func restBlock(games []core.DigestGame, collapsed bool) string {
	var b strings.Builder
	if collapsed {
		fmt.Fprintf(&b, "<blockquote expandable><b>%s</b>\n", restHeading(len(games)))
	} else {
		fmt.Fprintf(&b, "<blockquote><b>%s</b>\n", restHeading(len(games)))
	}

	width := codeWidth(games)
	for _, game := range games {
		winner, loser := sides(game.Game)
		fmt.Fprintf(&b, "\n<code>%-*s %3d : %-3d %s</code>",
			width, esc(string(winner.Team.Code)), winner.Score, loser.Score, esc(string(loser.Team.Code)))

		if top := game.Stats.TopScorers(1); len(top) > 0 {
			fmt.Fprintf(&b, " %s %d", esc(lastName(top[0].Player.Name)), top[0].Points)
		}
	}

	b.WriteString("</blockquote>")
	return b.String()
}

func restHeading(n int) string {
	return "Ещё " + plural(n, "матч", "матча", "матчей")
}

func codeWidth(games []core.DigestGame) int {
	width := 2
	for _, game := range games {
		for _, side := range []core.TeamScore{game.Game.Home, game.Game.Away} {
			width = max(width, len(side.Team.Code))
		}
	}
	return width
}

func sides(game core.Game) (core.TeamScore, core.TeamScore) {
	if game.Away.Score > game.Home.Score {
		return game.Away, game.Home
	}
	return game.Home, game.Away
}

func lastName(full string) string {
	trimmed := strings.TrimSpace(full)
	if i := strings.LastIndex(trimmed, " "); i >= 0 {
		if last := trimmed[i+1:]; !isSuffix(last) {
			return last
		}
		if j := strings.LastIndex(trimmed[:i], " "); j >= 0 {
			return trimmed[j+1:]
		}
		return trimmed
	}
	return trimmed
}

func isSuffix(s string) bool {
	switch strings.TrimSuffix(s, ".") {
	case "Jr", "Sr", "II", "III", "IV":
		return true
	}
	return false
}

func plural(n int, one, few, many string) string {
	return fmt.Sprintf("%d %s", n, pluralForm(n, one, few, many))
}

func pluralForm(n int, one, few, many string) string {
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}
