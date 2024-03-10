package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	IndexerDuration *prometheus.HistogramVec
	IndexerErrors   *prometheus.CounterVec
	IndexerRequests *prometheus.CounterVec
	CacheHits       *prometheus.CounterVec
	CacheMisses     *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		IndexerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "indexer_duration_seconds",
			Help:    "Duration of indexer requests",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 20, 30},
		}, []string{"indexer"}),
		IndexerErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "indexer_errors_total",
			Help: "Number of indexer errors",
		}, []string{"indexer"}),
		IndexerRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "indexer_requests_total",
			Help: "Number of indexer requests",
		}, []string{"indexer"}),
		CacheHits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Number of cache hits",
		}, []string{"cache"}),
		CacheMisses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Number of cache misses",
		}, []string{"cache"}),
	}
}

func (m *Metrics) Register() {
	prometheus.MustRegister(m.IndexerDuration)
	prometheus.MustRegister(m.IndexerErrors)
	prometheus.MustRegister(m.IndexerRequests)
	prometheus.MustRegister(m.CacheHits)
	prometheus.MustRegister(m.CacheMisses)
}
