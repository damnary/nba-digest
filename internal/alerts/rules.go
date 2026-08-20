package alerts

import "github.com/damnary/nba-digest/internal/core"

func Interested(subs []core.Subscriber) []core.Subscriber {
	var out []core.Subscriber
	for _, sub := range subs {
		if sub.AlertsOn {
			out = append(out, sub)
		}
	}
	return out
}
