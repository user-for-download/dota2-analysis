package dotaconstants

import (
	"context"
	"fmt"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich"
)

type HeroesSource struct {
	BaseURL string
	Writer  enrich.HeroesWriter
	HTTP    enrich.HTTPClient
}

func NewHeroesSource(baseURL string, w enrich.HeroesWriter, httpClient ...enrich.HTTPClient) *HeroesSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	h := enrich.HTTPClient(nil)
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &HeroesSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *HeroesSource) Name() string        { return "dotaconstants:heroes" }
func (s *HeroesSource) Critical() bool     { return true }
func (s *HeroesSource) DependsOn() []string { return nil }

func (s *HeroesSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	m, err := fetchJSON[map[string]heroJSON](ctx, client, s.BaseURL, "heroes.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	refs := make([]enrich.HeroRef, 0, len(m))
	for _, h := range m {
		refs = append(refs, enrich.HeroRef{
			ID:            h.ID,
			Name:          h.Name,
			LocalizedName: h.LocalizedName,
			PrimaryAttr:   h.PrimaryAttr,
			AttackType:    h.AttackType,
			Roles:         h.Roles,
			Legs:          h.Legs,
		})
	}

	if s.Writer == nil {
		return fmt.Errorf("heroes: writer not configured")
	}
	return s.Writer.UpsertHeroes(ctx, refs)
}

type heroJSON struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	LocalizedName string   `json:"localized_name"`
	PrimaryAttr   string   `json:"primary_attr"`
	AttackType    string   `json:"attack_type"`
	Roles         []string `json:"roles"`
	Legs          int      `json:"legs"`
}

var _ enrich.RunSource = (*HeroesSource)(nil)