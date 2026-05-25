package proxyclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/transport"
)

type RoundTripper struct {
	pool    proxy.Pool
	hold    time.Duration
	direct  bool
	timeout time.Duration
}

func NewRoundTripper(pool proxy.Pool, hold time.Duration, allowDirect bool, timeout time.Duration) *RoundTripper {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &RoundTripper{
		pool:    pool,
		hold:    hold,
		direct:  allowDirect,
		timeout: timeout,
	}
}

func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	lease, err := rt.pool.Acquire(req.Context(), rt.hold)
	if err != nil && err != proxy.ErrNoProxy {
		return nil, err
	}
	if lease != nil {
		defer func() { _ = lease.Release(context.WithoutCancel(req.Context())) }()
	}

	var httpClient *http.Client

	if lease != nil && lease.URL != "" {
		pu, err := url.Parse(lease.URL)
		if err != nil {
			lease.MarkFailure(req.Context(), err)
			return nil, fmt.Errorf("parse proxy url: %w", err)
		}
		tr, err := transport.For(pu, rt.timeout)
		if err != nil {
			lease.MarkFailure(req.Context(), err)
			return nil, fmt.Errorf("proxy transport: %w", err)
		}
		httpClient = &http.Client{Transport: tr, Timeout: rt.timeout}
		defer tr.CloseIdleConnections()
	} else if !rt.direct {
		return nil, proxy.ErrNoProxy
	} else {
		httpClient = &http.Client{Timeout: rt.timeout}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if lease != nil {
			lease.MarkFailure(req.Context(), err)
		}
		return nil, err
	}

	if resp.StatusCode >= 500 || resp.StatusCode == 429 {
		err := fmt.Errorf("http %d", resp.StatusCode)
		if lease != nil {
			lease.MarkFailure(req.Context(), err)
		}
		return nil, err
	}

	if lease != nil && resp.StatusCode < 400 {
		lease.MarkSuccess(req.Context())
	}

	return resp, nil
}

var _ http.RoundTripper = (*RoundTripper)(nil)