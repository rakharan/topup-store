package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Metrics struct {
	mu              sync.Mutex
	requestsTotal   map[string]int64
	requestDuration map[string][]time.Duration
}

const maxDurationEntries = 1000

func NewMetrics() *Metrics {
	return &Metrics{
		requestsTotal:   make(map[string]int64),
		requestDuration: make(map[string][]time.Duration),
	}
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		key := fmt.Sprintf("%s %s %d", r.Method, r.URL.Path, rw.statusCode)

		m.mu.Lock()
		m.requestsTotal[key]++
		durations := m.requestDuration[key]
		durations = append(durations, time.Since(start))
		if len(durations) > maxDurationEntries {
			durations = durations[len(durations)-maxDurationEntries:]
		}
		m.requestDuration[key] = durations
		m.mu.Unlock()
	})
}

func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		w.Write([]byte("# HELP http_requests_total Total number of HTTP requests\n"))
		w.Write([]byte("# TYPE http_requests_total counter\n"))
		for key, count := range m.requestsTotal {
			w.Write([]byte(fmt.Sprintf("http_requests_total{route=\"%s\"} %d\n", key, count)))
		}

		w.Write([]byte("# HELP http_request_duration_seconds HTTP request duration in seconds\n"))
		w.Write([]byte("# TYPE http_request_duration_seconds histogram\n"))
		for key, durations := range m.requestDuration {
			if len(durations) == 0 {
				continue
			}
			var total time.Duration
			for _, d := range durations {
				total += d
			}
			avg := total.Seconds() / float64(len(durations))
			w.Write([]byte(fmt.Sprintf("http_request_duration_seconds_avg{route=\"%s\"} %.4f\n", key, avg)))
		}
	}
}

func (m *Metrics) GetStats() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := make(map[string]any)
	for key, count := range m.requestsTotal {
		stats[key] = map[string]any{
			"total": count,
		}
	}
	return stats
}

type metricsRW struct {
	http.ResponseWriter
	statusCode int
}

func (mw *metricsRW) WriteHeader(code int) {
	mw.statusCode = code
	mw.ResponseWriter.WriteHeader(code)
}

func (mw *metricsRW) Write(b []byte) (int, error) {
	return mw.ResponseWriter.Write(b)
}

func (mw *metricsRW) Status() int {
	return mw.statusCode
}

func (mw *metricsRW) Written() int64 {
	return int64(0)
}

func NewMetricsResponseWriter(w http.ResponseWriter) *metricsRW {
	return &metricsRW{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

type PrometheusMetrics struct {
	mu            sync.Mutex
	counters      map[string]int64
	histograms    map[string][]float64
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
			for _, v := range values {
				sum += v
			}
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
		mw := NewMetricsResponseWriter(w)
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
