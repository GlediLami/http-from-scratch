package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var histBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

/*
*
* Three metrics, which is the standard minimum for an HTTP service:

http_requests_total — a counter, labelled by method, route pattern, status. "How many requests, split by outcome."
http_request_duration_seconds — a histogram, so you can compute p95/p99 latency later. "How long did requests take."
http_in_flight_requests — a gauge. "How many requests are being served right now."
*/
type metrics struct {
	mu       sync.Mutex
	reqs     map[string]uint64   // "method|path|status" -> count
	durCount map[string]uint64   // path -> number of observations
	durSum   map[string]float64  // path -> sum of seconds
	durBkt   map[string][]uint64 // path -> per-bucket counts (not yet cumulative)
	inflight int64
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(c int) {
	s.status = c
	s.ResponseWriter.WriteHeader(c)
}

// base constructor
func newMetrics() *metrics {
	return &metrics{
		reqs:     map[string]uint64{},
		durCount: map[string]uint64{},
		durSum:   map[string]float64{},
		durBkt:   map[string][]uint64{},
	}
}

func (m *metrics) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&m.inflight, 1)
		defer atomic.AddInt64(&m.inflight, -1)

		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		start := time.Now()
		next.ServeHTTP(rec, r)

		path := r.Pattern // e.g. "GET /users/{id}"

		if i := strings.IndexByte(path, ' '); i >= 0 {
			path = path[i+1:] // keep just "/users/{id}"
		}

		if path == "" {
			path = "other" // 404 / 405
		}

		m.observe(r.Method, path, rec.status, time.Since(start))
	})
}

func (m *metrics) observe(method, path string, status int, d time.Duration) {
	m.mu.Lock()

	defer m.mu.Unlock()

	m.reqs[method+"|"+path+"|"+strconv.Itoa(status)]++

	sec := d.Seconds()
	m.durCount[path]++
	m.durSum[path] += sec
	bkt := m.durBkt[path]

	if bkt == nil {
		bkt = make([]uint64, len(histBuckets))
		m.durBkt[path] = bkt
	}
	for i, b := range histBuckets {
		if sec <= b {
			bkt[i]++ // the smallest bucket this observation falls into
			break
		}
	}
}

func (m *metrics) handler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	fmt.Fprintln(w, "# HELP http_requests_total Total number of HTTP requests.")
	fmt.Fprintln(w, "# TYPE http_requests_total counter")

	keys := make([]string, 0, len(m.reqs))
	for k := range m.reqs {
		keys = append(keys, k)
	}

	sort.Strings(keys) // stable output is easier to eyeball and diff
	for _, k := range keys {
		p := strings.SplitN(k, "|", 3)
		fmt.Fprintf(w, "http_requests_total{method=%q,path=%q,status=%q} %d\n",
			p[0], p[1], p[2], m.reqs[k])
	}

	fmt.Fprintln(w, "# HELP http_request_duration_seconds Request duration in seconds.")
	fmt.Fprintln(w, "# TYPE http_request_duration_seconds histogram")
	paths := make([]string, 0, len(m.durCount))

	for p := range m.durCount {
		paths = append(paths, p)
	}

	sort.Strings(paths)
	for _, p := range paths {
		var cum uint64
		for i, b := range histBuckets {
			cum += m.durBkt[p][i] // histogram buckets are cumulative on the wire
			fmt.Fprintf(w, "http_request_duration_seconds_bucket{path=%q,le=%q} %d\n",
				p, strconv.FormatFloat(b, 'g', -1, 64), cum)
		}
		fmt.Fprintf(w, "http_request_duration_seconds_bucket{path=%q,le=\"+Inf\"} %d\n", p, m.durCount[p])
		fmt.Fprintf(w, "http_request_duration_seconds_sum{path=%q} %g\n", p, m.durSum[p])
		fmt.Fprintf(w, "http_request_duration_seconds_count{path=%q} %d\n", p, m.durCount[p])
	}

	fmt.Fprintln(w, "# HELP http_in_flight_requests Requests currently being served.")
	fmt.Fprintln(w, "# TYPE http_in_flight_requests gauge")
	fmt.Fprintf(w, "http_in_flight_requests %d\n", atomic.LoadInt64(&m.inflight))
}
