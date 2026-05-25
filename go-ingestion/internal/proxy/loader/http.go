package loader

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/proxy/transport"
)

type HTTPSource struct {
	URL      string
	Client   *http.Client
	ProxyURL string
	Timeout  time.Duration
}

func (h HTTPSource) Name() string { return "http:" + h.URL }

func (h HTTPSource) Load(ctx context.Context) ([]string, error) {
	if h.URL == "" {
		return nil, fmt.Errorf("URL is empty")
	}

	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	if h.ProxyURL != "" {
		pu, err := url.Parse(h.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy url: %w", err)
		}
		tr, err := transport.For(pu, timeout)
		if err != nil {
			return nil, fmt.Errorf("proxy transport: %w", err)
		}
		defer tr.CloseIdleConnections()
		client = &http.Client{Transport: tr, Timeout: timeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go-dota2/proxyloader")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch remote list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("remote list status %d", resp.StatusCode)
	}

	var out []string
	s := bufio.NewScanner(resp.Body)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, s.Err()
}