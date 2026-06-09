package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "radio_review_http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "route", "status"},
	)

	HTTPRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "radio_review_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)

	RecordingJobRunsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "radio_review_recording_job_runs_total",
			Help: "Total number of recording job executions.",
		},
		[]string{"job"},
	)

	RecordingJobFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "radio_review_recording_job_failures_total",
			Help: "Total number of recording job failures.",
		},
		[]string{"job", "reason"},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDurationSeconds,
		RecordingJobRunsTotal,
		RecordingJobFailuresTotal,
	)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		route := r.URL.Path
		status := strconv.Itoa(rec.status)
		HTTPRequestsTotal.WithLabelValues(r.Method, route, status).Inc()
		HTTPRequestDurationSeconds.WithLabelValues(r.Method, route, status).Observe(time.Since(start).Seconds())
	})
}

func IncRecordingJobRun(job string) {
	RecordingJobRunsTotal.WithLabelValues(job).Inc()
}

func IncRecordingJobFailure(job, reason string) {
	RecordingJobFailuresTotal.WithLabelValues(job, reason).Inc()
}
