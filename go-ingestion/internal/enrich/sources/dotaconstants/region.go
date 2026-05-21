package dotaconstants

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/user-for-download/go-dota2/internal/enrich"
)

type RegionsSource struct {
	BaseURL string
	Writer  enrich.RegionsWriter
	HTTP    enrich.HTTPClient
}

func NewRegionsSource(baseURL string, w enrich.RegionsWriter, httpClient ...enrich.HTTPClient) *RegionsSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	var h enrich.HTTPClient
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &RegionsSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *RegionsSource) Name() string        { return "dotaconstants:region" }
func (s *RegionsSource) Critical() bool       { return false }
func (s *RegionsSource) DependsOn() []string   { return nil }

func (s *RegionsSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	raw, err := fetchJSON[map[string]string](ctx, client, s.BaseURL, "region.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	refs := make([]enrich.RegionRef, 0, len(raw))
	for k, name := range raw {
		id, err := strconv.Atoi(strings.TrimSpace(k))
		if err != nil || name == "" {
			continue
		}
		refs = append(refs, enrich.RegionRef{ID: id, Name: name})
	}
	return s.Writer.UpsertRegions(ctx, refs)
}

var _ enrich.RunSource = (*RegionsSource)(nil)