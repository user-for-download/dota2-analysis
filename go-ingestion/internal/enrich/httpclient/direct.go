package httpclient

import (
	"context"
	"net/http"
	"time"
)

type DirectClient struct {
	client *http.Client
}

func NewDirect(timeout ...time.Duration) *DirectClient {
	t := 30 * time.Second
	if len(timeout) > 0 && timeout[0] > 0 {
		t = timeout[0]
	}
	return &DirectClient{
		client: &http.Client{Timeout: t},
	}
}

func (d *DirectClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go-dota2/enricher")
	return d.client.Do(req)
}