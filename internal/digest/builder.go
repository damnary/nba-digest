package digest

import (
	"github.com/damnary/nba-digest/internal/core"
)

func Build(sub core.Subscriber, subscriptions []core.Subscription, games []core.Game, stats map[core.GameID]core.GameStats, day core.Day) core.Digest {
	followed := make(map[core.TeamCode]bool, len(subscriptions))
	for _, s := range subscriptions {
		followed[s.Team] = true
	}

	digest := core.Digest{Subscriber: sub.ID, Day: day}

	for _, game := range games {
		if !game.IsFinal() {
			continue
		}
		if digest.League == "" {
			digest.League = game.League
		}

		digest.Games = append(digest.Games, core.DigestGame{
			Game:     game,
			Stats:    stats[game.ID],
			Featured: followed[game.Home.Team.Code] || followed[game.Away.Team.Code],
		})
	}
	return digest
}
