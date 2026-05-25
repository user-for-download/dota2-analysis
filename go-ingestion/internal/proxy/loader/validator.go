package loader

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/transport"
)

type Validator struct {
	CanaryURL      string
	Timeout       time.Duration
	RequireIPBody bool
}

func (v Validator) Probe(ctx context.Context, proxyRaw string) error {
	if proxyRaw == "" {
		return fmt.Errorf("empty proxy URL")
	}

	pu, err := url.Parse(proxyRaw)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	timeout := v.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	tr, err := transport.For(pu, timeout)
	if err != nil {
		return fmt.Errorf("transport: %w", err)
	}
	defer tr.CloseIdleConnections()

	client := &http.Client{Transport: tr, Timeout: timeout}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, http.MethodGet, v.CanaryURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "go-dota2/proxyloader")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("canary status %d", resp.StatusCode)
	}

	if v.RequireIPBody {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		ip := net.ParseIP(strings.TrimSpace(string(body)))
		if ip == nil {
			return fmt.Errorf("canary returned non-IP body: %q", strings.TrimSpace(string(body)))
		}
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))

	return nil
}