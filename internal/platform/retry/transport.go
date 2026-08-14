package retry

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type Transport struct {
	Next   http.RoundTripper
	Policy Policy
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}

	if req.Body != nil {
		return next.RoundTrip(req)
	}

	var (
		resp *http.Response
		err  error
	)
	for attempt := range t.Policy.attempts() {
		resp, err = next.RoundTrip(req)

		switch {
		case err != nil:
		case !retryableStatus(resp.StatusCode):
			return resp, nil
		}

		if attempt == t.Policy.attempts()-1 {
			break
		}

		wait := t.Policy.Backoff(attempt)
		if resp != nil {
			if after, ok := retryAfter(resp); ok {
				wait = after
			}
			drain(resp)
		}

		select {
		case <-time.After(wait):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}

	if err != nil {
		return nil, fmt.Errorf("after %d attempts: %w", t.Policy.attempts(), err)
	}
	return resp, nil
}

func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
}

func retryAfter(resp *http.Response) (time.Duration, bool) {
	raw := resp.Header.Get("Retry-After")
	if raw == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if at, err := http.ParseTime(raw); err == nil {
		if d := time.Until(at); d > 0 {
			return d, true
		}
	}
	return 0, false
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}
