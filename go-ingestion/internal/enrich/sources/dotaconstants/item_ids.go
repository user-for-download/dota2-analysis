package dotaconstants

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich"
)

type ItemIDsSource struct {
	BaseURL string
	Writer  enrich.ItemIDsWriter
	HTTP    enrich.HTTPClient
}

func NewItemIDsSource(baseURL string, w enrich.ItemIDsWriter, httpClient ...enrich.HTTPClient) *ItemIDsSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	var h enrich.HTTPClient
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &ItemIDsSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *ItemIDsSource) Name() string        { return "dotaconstants:item_ids" }
func (s *ItemIDsSource) Critical() bool       { return false }
func (s *ItemIDsSource) DependsOn() []string  { return []string{"dotaconstants:items"} }

func (s *ItemIDsSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	raw, err := fetchJSON[map[string]string](ctx, client, s.BaseURL, "item_ids.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	refs := make([]enrich.ItemIDRef, 0, len(raw))
	for k, name := range raw {
		id, err := strconv.Atoi(strings.TrimSpace(k))
		if err != nil || id < 0 || name == "" {
			continue
		}
		refs = append(refs, enrich.ItemIDRef{ID: id, Key: name})
	}
	return s.Writer.UpsertItemIDs(ctx, refs)
}

var _ enrich.RunSource = (*ItemIDsSource)(nil)