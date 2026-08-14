package live

import (
	"slices"

	"github.com/damnary/nba-digest/internal/core"
)

func Aggregate(box core.GameStats, plays []core.Play) core.GameStats {
	clutch := make(map[core.PlayerID]int)
	margin := 0

	for _, p := range plays {
		if !p.IsClutch() {
			margin = p.Margin()
			continue
		}
		if p.Points > 0 && p.Player.ID != "" {
			clutch[p.Player.ID] += p.Points
		}
	}

	out := box
	out.ClutchMargin = margin
	out.Lines = slices.Clone(box.Lines)
	for i := range out.Lines {
		out.Lines[i].ClutchPoints = clutch[out.Lines[i].Player.ID]
	}
	return out
}
