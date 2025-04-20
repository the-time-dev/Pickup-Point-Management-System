package http_api

import "github.com/prometheus/client_golang/prometheus"

var httpRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	},
	[]string{"method", "endpoint"},
)

var httpRequestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: []float64{0.1, 0.5, 1, 2, 5}, // Интервалы времени
	},
	[]string{"method", "endpoint"},
)

var pvzCreatedTotal = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "pvz_created_total",
		Help: "Total number of created pickup points (PVZ)",
	},
)

var receptionsTotal = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "receptions_created_total",
		Help: "Total number of created order acceptances",
	},
)

var productAddedTotal = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "product_added_total",
		Help: "Total number of added products",
	},
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(pvzCreatedTotal)
	prometheus.MustRegister(receptionsTotal)
	prometheus.MustRegister(productAddedTotal)
}
