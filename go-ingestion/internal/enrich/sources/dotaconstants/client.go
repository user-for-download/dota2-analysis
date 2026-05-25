package dotaconstants

import (
	"context"
	"fmt"
	"strings"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich/jsonutil"
)

const DefaultBaseURL = "https://raw.githubusercontent.com/odota/dotaconstants/master/build"

func fetchJSON[T any](ctx context.Context, http enrich.HTTPClient, base, file string) (T, error) {
	var zero T
	if http == nil {
		return zero, fmt.Errorf("http client is nil")
	}
	url := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(file, "/")
	resp, err := http.Get(ctx, url)
	if err != nil {
		return zero, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := jsonutil.ReadBody(resp)
	if err != nil {
		return zero, fmt.Errorf("read %s: %w", url, err)
	}
	out, err := jsonutil.Unmarshal[T](body)
	if err != nil {
		return zero, fmt.Errorf("decode %s: %w", url, err)
	}
	return out, nil
}