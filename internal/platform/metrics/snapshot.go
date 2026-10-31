package metrics

import (
	"time"

	dto "github.com/prometheus/client_model/go"
)

type Snapshot struct {
	TakenAt     time.Time
	LastScan    time.Time
	LastUpdates time.Time
	GamesWated  int
	Events      map[string]float64
	Deliveries  map[string]float64
	Requests    map[string]float64
}

func (m *Metrics) Snapshot() Snapshot {
	snap := Snapshot{
		TakenAt:    m.now(),
		Events:     map[string]float64{},
		Deliveries: map[string]float64{},
		Requests:   map[string]float64{},
	}

	families, err := m.registry.Gather()
	if err != nil {
		return snap
	}

	for _, family := range families {
		for _, metric := range family.GetMetric() {
			switch family.GetName() {
			case namespace + "_scan_last_success_timestamp_seconds":
				snap.LastScan = unix(metric.GetGauge().GetValue())
			case namespace + "_updates_last_success_timestamp_seconds":
				snap.LastUpdates = unix(metric.GetGauge().GetValue())
			case namespace + "_games_watched":
				snap.GamesWated = int(metric.GetGauge().GetValue())
			case namespace + "_events_detected_total":
				snap.Events[label(metric, "kind")] += metric.GetCounter().GetValue()
			case namespace + "_deliveries_total":
				key := label(metric, "channel") + "/" + label(metric, "result")
				snap.Deliveries[key] += metric.GetCounter().GetValue()
			case namespace + "_outbound_requests_total":
				snap.Requests[label(metric, "status")] += metric.GetCounter().GetValue()
			}
		}
	}
	return snap
}

func (s Snapshot) Since(previous Snapshot) Snapshot {
	out := s
	out.Events = diff(s.Events, previous.Events)
	out.Deliveries = diff(s.Deliveries, previous.Deliveries)
	out.Requests = diff(s.Requests, previous.Requests)
	return out
}

func diff(now, before map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(now))
	for k, v := range now {
		if d := v - before[k]; d > 0 {
			out[k] = d
		}
	}
	return out
}

func label(m *dto.Metric, name string) string {
	for _, pair := range m.GetLabel() {
		if pair.GetName() == name {
			return pair.GetValue()
		}
	}
	return ""
}

func unix(seconds float64) time.Time {
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(seconds), 0)
}
