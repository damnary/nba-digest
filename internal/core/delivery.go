package core

import "time"

type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "pending"
	DeliverySent    DeliveryStatus = "sent"
	DeliverySkipped DeliveryStatus = "skipped"
	DeliveryFailed  DeliveryStatus = "failed"
)

const (
	CatchUpWindow       = 15 * time.Minute
	MaxDeliveryAttempts = 5
)

func (e Event) IsStale(now time.Time) bool {
	return now.Sub(e.OccurredAt) > CatchUpWindow
}
