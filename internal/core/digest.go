package core

import (
	"fmt"
	"time"
)

type Day struct {
	Year  int
	Month time.Month
	Day   int
}

func DayOf(t time.Time) Day {
	y, m, d := t.Date()
	return Day{Year: y, Month: m, Day: d}
}

func (d Day) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

func (d Day) Prev() Day {
	return DayOf(time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1))
}

func (d Day) Bounds(loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.UTC
	}
	start := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, loc)
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

type DigestGame struct {
	Game     Game
	Stats    GameStats
	Featured bool
}

type Digest struct {
	Subscriber SubscriberID
	League     League
	Day        Day
	Games      []DigestGame
}

func (d Digest) IsEmpty() bool { return len(d.Games) == 0 }

func (d Digest) FeaturedGames() []DigestGame {
	return d.filter(true)
}

func (d Digest) RestGames() []DigestGame {
	return d.filter(false)
}

func (d Digest) filter(featured bool) []DigestGame {
	var out []DigestGame
	for _, g := range d.Games {
		if g.Featured == featured {
			out = append(out, g)
		}
	}
	return out
}
