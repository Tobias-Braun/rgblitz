package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/tobiasmarkusbraun/rgblitz/internal/api"
	"github.com/tobiasmarkusbraun/rgblitz/internal/metrics"
)

// TestInstrument_LabelsByRoutePattern checks requests are labelled by chi's
// route pattern (never the raw path, which carries player/job IDs), and that
// requests no route matched share one "unmatched" label. None of these
// requests reach a store, so an empty Handler suffices.
func TestInstrument_LabelsByRoutePattern(t *testing.T) {
	router := api.NewRouter(&api.Handler{})

	cases := []struct {
		method, path          string
		route, status, reason string
	}{
		{"GET", "/health", "/health", "200", "implicit 200 with no body"},
		{"GET", "/api/players/LUC2110/today/jobs/not-a-number", "/api/players/{id}/today/jobs/{jobId}", "400", "pattern, not path"},
		{"GET", "/no/such/page", "unmatched", "404", "unmatched route"},
	}
	for _, c := range cases {
		counter := metrics.HTTPRequests.WithLabelValues(c.route, c.method, c.status)
		before := testutil.ToFloat64(counter)

		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(c.method, c.path, nil))

		if got := testutil.ToFloat64(counter) - before; got != 1 {
			t.Errorf("%s %s (%s): rgblitz_http_requests_total{route=%q,status=%q} grew by %v, want 1",
				c.method, c.path, c.reason, c.route, c.status, got)
		}
	}
}

func TestInstrument_CapsUnknownMethods(t *testing.T) {
	router := api.NewRouter(&api.Handler{})
	counter := metrics.HTTPRequests.WithLabelValues("unmatched", "other", "405")
	before := testutil.ToFloat64(counter)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("BREW", "/coffee", nil))

	if got := testutil.ToFloat64(counter) - before; got != 1 {
		t.Errorf("unknown method counted %v times under method=\"other\", want 1", got)
	}
}
