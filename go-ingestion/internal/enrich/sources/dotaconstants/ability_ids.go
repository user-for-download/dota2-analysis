package dotaconstants

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich"
)

type AbilityIDsSource struct {
	BaseURL string
	Writer  enrich.AbilityIDsWriter
	HTTP    enrich.HTTPClient
}

func NewAbilityIDsSource(baseURL string, w enrich.AbilityIDsWriter, httpClient ...enrich.HTTPClient) *AbilityIDsSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	h := enrich.HTTPClient(nil)
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &AbilityIDsSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *AbilityIDsSource) Name() string   { return "dotaconstants:ability_ids" }
func (s *AbilityIDsSource) Critical() bool { return false }
func (s *AbilityIDsSource) DependsOn() []string { return []string{"dotaconstants:abilities"} }

func (s *AbilityIDsSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	raw, err := fetchJSON[map[string]string](ctx, client, s.BaseURL, "ability_ids.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	refs := make([]enrich.AbilityIDRef, 0, len(raw))
	for idStr, name := range raw {
		id, err := strconv.Atoi(strings.TrimSpace(idStr))
		if err != nil || id <= 0 || name == "" {
			continue
		}
		refs = append(refs, enrich.AbilityIDRef{ID: id, Name: name})
	}

	if err := s.Writer.UpsertAbilityIDs(ctx, refs); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	return nil
}

var _ enrich.RunSource = (*AbilityIDsSource)(nil)