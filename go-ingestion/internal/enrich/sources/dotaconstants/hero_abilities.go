package dotaconstants

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/user-for-download/go-dota2/internal/enrich"
)

type HeroAbilitiesSource struct {
	BaseURL string
	Writer  enrich.HeroAbilitiesWriter
	HTTP    enrich.HTTPClient
}

func NewHeroAbilitiesSource(baseURL string, w enrich.HeroAbilitiesWriter, httpClient ...enrich.HTTPClient) *HeroAbilitiesSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	h := enrich.HTTPClient(nil)
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &HeroAbilitiesSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *HeroAbilitiesSource) Name() string   { return "dotaconstants:hero_abilities" }
func (s *HeroAbilitiesSource) Critical() bool { return true }
func (s *HeroAbilitiesSource) DependsOn() []string { return []string{"dotaconstants:heroes"} }

func (s *HeroAbilitiesSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	raw, err := fetchJSON[map[string]heroAbilitiesJSON](ctx, client, s.BaseURL, "hero_abilities.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	var (
		abilities = make([]enrich.HeroAbilityRef, 0, len(raw)*6)
		talents   = make([]enrich.HeroTalentRef, 0, len(raw)*8)
		facets    = make([]enrich.HeroFacetRef, 0, len(raw)*2)
	)

	for hero, ha := range raw {
		for slot, rawAb := range ha.Abilities {
			ab := flattenAbilitySlot(rawAb)
			if ab == "" || ab == "generic_hidden" {
				continue
			}
			abilities = append(abilities, enrich.HeroAbilityRef{
				HeroName: hero, Slot: slot, Ability: ab,
			})
		}
		for _, t := range ha.Talents {
			if t.Name == "" || t.Level <= 0 {
				continue
			}
			talents = append(talents, enrich.HeroTalentRef{
				HeroName: hero, Ability: t.Name, Level: t.Level,
			})
		}
		for _, f := range ha.Facets {
			facets = append(facets, enrich.HeroFacetRef{
				HeroName:    hero,
				Slot:        f.ID,
				Name:        f.Name,
				Title:       f.Title,
				Description: f.Description,
				Icon:        f.Icon,
				Color:       f.Color,
				GradientID:  f.GradientID,
				Deprecated:  isDeprecated(f.Deprecated),
			})
		}
	}

	if len(abilities) > 0 {
		if err := s.Writer.UpsertHeroAbilities(ctx, abilities); err != nil {
			return fmt.Errorf("hero_abilities: %w", err)
		}
	}
	if len(talents) > 0 {
		if err := s.Writer.UpsertHeroTalents(ctx, talents); err != nil {
			return fmt.Errorf("hero_talents: %w", err)
		}
	}
	if len(facets) > 0 {
		if err := s.Writer.UpsertHeroFacets(ctx, facets); err != nil {
			return fmt.Errorf("hero_facets: %w", err)
		}
	}
	return nil
}

type heroAbilitiesJSON struct {
	Abilities []json.RawMessage `json:"abilities"`
	Talents   []talentEntry     `json:"talents"`
	Facets    []facetEntry      `json:"facets"`
}

type talentEntry struct {
	Name  string `json:"name"`
	Level int    `json:"level"`
}

type facetEntry struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Icon        string          `json:"icon"`
	Color       string          `json:"color"`
	GradientID  int             `json:"gradient_id"`
	Deprecated  json.RawMessage `json:"deprecated"`
}

func flattenAbilitySlot(raw json.RawMessage) string {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		return arr[0]
	}
	return ""
}

func isDeprecated(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" || s == "false" || s == `""` {
		return false
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return strings.EqualFold(str, "true") || str != ""
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		return len(arr) > 0
	}
	return false
}

var _ enrich.RunSource = (*HeroAbilitiesSource)(nil)