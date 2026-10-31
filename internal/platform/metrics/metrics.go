package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "nba_digest"

type Metrics struct {
	registry *prometheus.Registry

	scanAt      prometheus.Gauge
	updatesAt   prometheus.Gauge
	gamesWached prometheus.Gauge
	events      *prometheus.CounterVec
	deliveries  *prometheus.CounterVec
	requests    *prometheus.CounterVec
	latency     *prometheus.HistogramVec

	now func() time.Time
}

func New(version string) *Metrics {
	m := &Metrics{registry: prometheus.NewRegistry(), now: time.Now}

	build := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "build_info",
		Help:      "Version of the running binary.",
	}, []string{"version"})
	build.WithLabelValues(version).Set(1)

	m.scanAt = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "scan_last_success_timestamp_seconds",
		Help:      "When the scoreboard was last read without error.",
	})
	m.updatesAt = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "updates_last_success_timestamp_seconds",
		Help:      "When Telegram updates were last received without error.",
	})
	m.gamesWached = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "games_watched",
		Help:      "Games currently being followed play by play.",
	})
	m.events = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_detected_total",
		Help:      "Game events found by the detector.",
	}, []string{"kind"})
	m.deliveries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "deliveries_total",
		Help:      "Messages the service tried to put in front of a subscriber.",
	}, []string{"channel", "result"})
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "outbound_requests_total",
		Help:      "Outbound HTTP calls by host and status.",
	}, []string{"host", "status"})
	m.latency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "outbound_request_seconds",
		Help:      "How long outbound HTTP calls take.",
		Buckets:   []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
	}, []string{"host"})

	m.registry.MustRegister(build, m.scanAt, m.updatesAt, m.gamesWached,
		m.events, m.deliveries, m.requests, m.latency)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ScanSucceeded() {
	m.scanAt.Set(float64(m.now().Unix()))
}

func (m *Metrics) UpdatesReceived() {
	m.updatesAt.Set(float64(m.now().Unix()))
}

func (m *Metrics) GamesWatched(n int) {
	m.gamesWached.Set(float64(n))
}

func (m *Metrics) EventDetected(kind string) {
	m.events.WithLabelValues(kind).Inc()
}

func (m *Metrics) Delivered(channel, result string) {
	m.deliveries.WithLabelValues(channel, result).Inc()
}

func (m *Metrics) RequestDone(host, status string, took time.Duration) {
	m.requests.WithLabelValues(host, status).Inc()
	m.latency.WithLabelValues(host).Observe(took.Seconds())
}
