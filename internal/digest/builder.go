package digest

import (
	"github.com/damnary/nba-digest/internal/core"
)

type followedTeam struct {
	league core.League
	team   core.TeamCode
}

func Build(sub core.Subscriber, subscriptions []core.Subscription, games []core.Game, stats map[core.GameID]core.GameStats, day core.Day) core.Digest {
	followed := make(map[followedTeam]bool, len(subscriptions))
	for _, s := range subscriptions {
		followed[followedTeam{league: s.League, team: s.Team}] = true
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
			Game:  game,
			Stats: stats[game.ID],
			Featured: followed[followedTeam{league: game.League, team: game.Home.Team.Code}] ||
				followed[followedTeam{league: game.League, team: game.Away.Team.Code}],
		})
	}
	return digest
}
