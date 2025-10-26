package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	LogsProcessedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_logs_processed_total",
			Help: "Total number of logs processed",
		},
		[]string{"content_type"},
	)

	AnomaliesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_anomalies_total",
			Help: "Total number of anomalies detected",
		},
		[]string{"content_type"},
	)

	ProcessingLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "app_processing_latency_seconds",
			Help:    "Latency of log processing in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"content_type"},
	)
)

func Init() {
	prometheus.MustRegister(LogsProcessedTotal)
	prometheus.MustRegister(AnomaliesTotal)
	prometheus.MustRegister(ProcessingLatency)
}
