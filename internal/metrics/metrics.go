// Package metrics defines the Prometheus metrics exported by the API
// server and worker.
package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	URLsCreated      prometheus.Counter
	Redirects        *prometheus.CounterVec // result label: hit, miss, not_found, expired
	RedirectLatency  prometheus.Histogram
	CreateLatency    prometheus.Histogram
	DroppedEvents    prometheus.Counter
	ClickEventsBatch prometheus.Histogram // batch size per flush
	RateLimited      prometheus.Counter
}

func New(registerer prometheus.Registerer) *Metrics {
	m := &Metrics{
		URLsCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "urlshortener_urls_created_total",
			Help: "Total number of short URLs created",
		}),
		Redirects: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "urlshortener_redirects_total",
			Help: "Total number of redirect requests by result",
		}, []string{"result"}),
		RedirectLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "urlshortener_redirect_latency_seconds",
			Help:    "Redirect resolution latency",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		}),
		CreateLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "urlshortener_create_latency_seconds",
			Help:    "Shorten-URL creation latency",
			Buckets: []float64{.01, .05, .1, .2, .5, 1, 2},
		}),
		DroppedEvents: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "urlshortener_dropped_click_events_total",
			Help: "Click events dropped because the batch channel was full",
		}),
		ClickEventsBatch: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "urlshortener_click_events_batch_size",
			Help:    "Number of click events written per batch flush",
			Buckets: []float64{1, 10, 50, 100, 250, 500, 1000},
		}),
		RateLimited: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "urlshortener_rate_limited_total",
			Help: "Total number of requests rejected by the rate limiter",
		}),
	}

	registerer.MustRegister(
		m.URLsCreated, m.Redirects, m.RedirectLatency, m.CreateLatency,
		m.DroppedEvents, m.ClickEventsBatch, m.RateLimited,
	)
	return m
}
