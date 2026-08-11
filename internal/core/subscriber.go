package core

import (
	"fmt"
	"time"
)

type SubscriberID int64

type DailyTime struct {
	Hour   int
	Minute int
}

func ParseDailyTime(s string) (DailyTime, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return DailyTime{}, fmt.Errorf("parse daily time %q: %w", s, err)
	}
	return DailyTime{Hour: t.Hour(), Minute: t.Minute()}, nil
}

func (d DailyTime) String() string {
	return fmt.Sprintf("%02d:%02d", d.Hour, d.Minute)
}

func (d DailyTime) Matches(t time.Time) bool {
	return t.Hour() == d.Hour && t.Minute() == d.Minute
}

type Subscriber struct {
	ID        SubscriberID
	ChatID    int64
	Timezone  *time.Location
	DigestAt  DailyTime
	AlertsOn  bool
	CreatedAt time.Time
}

func (s Subscriber) LocalTime(now time.Time) time.Time {
	if s.Timezone == nil {
		return now.UTC()
	}
	return now.In(s.Timezone)
}

type Subscription struct {
	SubscriberID SubscriberID
	League       League
	Team         TeamCode
	CreatedAt    time.Time
}
