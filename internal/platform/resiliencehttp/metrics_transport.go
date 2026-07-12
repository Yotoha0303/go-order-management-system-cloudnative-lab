package resiliencehttp

import (
	"net/http"
	"strings"
	"time"

	platformmetrics "go-order-management-system/internal/platform/metrics"
)

type metricsRoundTripper struct {
	base http.RoundTripper
}

func ObserveTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &metricsRoundTripper{base: base}
}

func (transport *metricsRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	started := time.Now()
	response, err := transport.base.RoundTrip(request)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	outcome := "success"
	if err != nil {
		outcome = "transport_error"
	} else if status < 200 || status >= 300 {
		outcome = "http_error"
	}
	upstream := "unknown"
	if request != nil && request.URL != nil {
		upstream = strings.TrimSpace(request.URL.Host)
	}
	operation := "unmatched"
	if request != nil && request.URL != nil {
		operation = platformmetrics.RouteGroup(request.URL.Path)
	}
	platformmetrics.RecordHTTPClientAttempt(
		upstream,
		operation,
		outcome,
		status,
		shouldRetry(request.Context(), response, err),
		time.Since(started),
	)
	return response, err
}
