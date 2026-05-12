package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const maxDurationEntries = 1000

type PrometheusMetrics struct {
	mu         sync.Mutex
	counters   map[string]int64
	histograms map[string][]float64
}

func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		counters:   make(map[string]int64),
		histograms: make(map[string][]float64),
	}
}

func (pm *PrometheusMetrics) IncrementCounter(name string, labels map[string]string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	key := name + serializeLabels(labels)
	pm.counters[key]++
}

func (pm *PrometheusMetrics) ObserveHistogram(name string, labels map[string]string, value float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	key := name + serializeLabels(labels)
	values := pm.histograms[key]
	values = append(values, value)
	if len(values) > maxDurationEntries {
		values = values[len(values)-maxDurationEntries:]
	}
	pm.histograms[key] = values
}

func (pm *PrometheusMetrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pm.mu.Lock()
		defer pm.mu.Unlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		for key, count := range pm.counters {
			w.Write([]byte(fmt.Sprintf("%s %d\n", key, count)))
		}

		for key, values := range pm.histograms {
			if len(values) == 0 {
				continue
			}
			var sum float64
			buckets := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
			counts := make([]int, len(buckets))
			for _, v := range values {
				sum += v
				for i, b := range buckets {
					if v <= b {
						counts[i]++
					}
				}
			}
			for i, b := range buckets {
				w.Write([]byte(fmt.Sprintf("%s_bucket{le=\"%g\"} %d\n", key, b, counts[i])))
			}
			w.Write([]byte(fmt.Sprintf("%s_bucket{le=\"+Inf\"} %d\n", key, len(values))))
			w.Write([]byte(fmt.Sprintf("%s_sum %f\n", key, sum)))
			w.Write([]byte(fmt.Sprintf("%s_count %d\n", key, len(values))))
		}
	}
}

func serializeLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	result := "{"
	first := true
	for k, v := range labels {
		if !first {
			result += ","
		}
		result += fmt.Sprintf("%s=\"%s\"", k, v)
		first = false
	}
	result += "}"
	return result
}

type MetricsMiddleware struct {
	metrics *PrometheusMetrics
}

func NewMetricsMiddleware() *MetricsMiddleware {
	return &MetricsMiddleware{
		metrics: NewPrometheusMetrics(),
	}
}

func (mm *MetricsMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		mw := &ResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(mw, r)
		duration := time.Since(start).Seconds()

		mm.metrics.IncrementCounter("http_requests_total", map[string]string{
			"method": r.Method,
			"path":   r.URL.Path,
			"status": strconv.Itoa(mw.Status()),
		})
		mm.metrics.ObserveHistogram("http_request_duration_seconds", map[string]string{
			"method": r.Method,
			"path":   r.URL.Path,
		}, duration)
	})
}

func (mm *MetricsMiddleware) Handler() http.HandlerFunc {
	return mm.metrics.Handler()
}
