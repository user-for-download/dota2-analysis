package dotaconstants

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich"
)

type AbilitiesSource struct {
	BaseURL string
	Writer  enrich.AbilitiesWriter
	HTTP    enrich.HTTPClient
}

func NewAbilitiesSource(baseURL string, w enrich.AbilitiesWriter, httpClient ...enrich.HTTPClient) *AbilitiesSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	h := enrich.HTTPClient(nil)
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &AbilitiesSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *AbilitiesSource) Name() string   { return "dotaconstants:abilities" }
func (s *AbilitiesSource) Critical() bool { return true }
func (s *AbilitiesSource) DependsOn() []string { return nil }

func (s *AbilitiesSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	raw, err := fetchJSON[map[string]abilityJSON](ctx, client, s.BaseURL, "abilities.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	refs := make([]enrich.AbilityRef, 0, len(raw))
	for name, a := range raw {
		if name == "" || isSchemaAbility(name) {
			continue
		}
		refs = append(refs, enrich.AbilityRef{
			Name:        name,
			Localized:   a.DName,
			Description: a.Desc,
			Img:         a.Img,
			ManaCost:    rawToString(a.MC),
			Cooldown:    rawToString(a.CD),
			TargetTeam:  rawToString(a.TargetTeam),
			Behavior:    coerceArrayJSON(a.Behavior),
			Attrib:      coerceArrayJSON(a.Attrib),
			IsTalent:    false,
		})
	}

	if err := s.Writer.UpsertAbilities(ctx, refs); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	return nil
}

func isSchemaAbility(name string) bool {
	return name == "special_bonus_attributes" || name == "dota_base_ability"
}

type abilityJSON struct {
	DName      string          `json:"dname"`
	Desc       string          `json:"desc"`
	Img        string          `json:"img"`
	MC         json.RawMessage `json:"mc"`
	CD         json.RawMessage `json:"cd"`
	TargetTeam json.RawMessage `json:"target_team"`
	Behavior   json.RawMessage `json:"behavior"`
	Attrib     json.RawMessage `json:"attrib"`
}

func rawToString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return strings.Join(arr, "/")
	}
	return ""
}

func coerceArrayJSON(raw json.RawMessage) json.RawMessage {
	t := strings.TrimSpace(string(raw))
	if t == "" || t == "null" {
		return json.RawMessage(`[]`)
	}
	if strings.HasPrefix(t, "[") || strings.HasPrefix(t, "{") {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return json.RawMessage(`[]`)
	}
	out, err := json.Marshal([]any{v})
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return out
}

var _ enrich.RunSource = (*AbilitiesSource)(nil)