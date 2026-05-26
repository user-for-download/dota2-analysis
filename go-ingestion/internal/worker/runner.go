package worker

import (
	"context"
	"net/http"
)

// HTTPDoer performs an HTTP request, returning the response.
type HTTPDoer interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}
