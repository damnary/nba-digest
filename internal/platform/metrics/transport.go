package metrics

import (
	"net/http"
	"strconv"
)

type Transport struct {
	Next    http.RoundTripper
	Metrics *Metrics
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	if t.Metrics == nil {
		return next.RoundTrip(req)
	}

	started := t.Metrics.now()
	resp, err := next.RoundTrip(req)
	took := t.Metrics.now().Sub(started)

	status := "error"
	if resp != nil {
		status = strconv.Itoa(resp.StatusCode)
	}
	t.Metrics.RequestDone(req.URL.Hostname(), status, took)

	return resp, err
}
