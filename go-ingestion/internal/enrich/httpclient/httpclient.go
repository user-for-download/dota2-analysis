package httpclient

import (
	"context"
	"net/http"
)

type HTTPClient interface {
	Get(ctx context.Context, url string) (*http.Response, error)
}