package dotaconstants

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich"
)

type GameModesSource struct {
	BaseURL string
	Writer  enrich.GameModesWriter
	HTTP    enrich.HTTPClient
}

func NewGameModesSource(baseURL string, w enrich.GameModesWriter, httpClient ...enrich.HTTPClient) *GameModesSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	var h enrich.HTTPClient
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &GameModesSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *GameModesSource) Name() string        { return "dotaconstants:game_mode" }
func (s *GameModesSource) Critical() bool       { return false }
func (s *GameModesSource) DependsOn() []string { return nil }

type gameModeJSON struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Balanced bool   `json:"balanced"`
}

func (s *GameModesSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	raw, err := fetchJSON[map[string]gameModeJSON](ctx, client, s.BaseURL, "game_mode.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	refs := make([]enrich.GameModeRef, 0, len(raw))
	for k, v := range raw {
		id := v.ID
		if id == 0 {
			n, err := strconv.Atoi(strings.TrimSpace(k))
			if err != nil {
				continue
			}
			id = n
		}
		if v.Name == "" {
			continue
		}
		refs = append(refs, enrich.GameModeRef{ID: id, Name: v.Name, Balanced: v.Balanced})
	}
	return s.Writer.UpsertGameModes(ctx, refs)
}

var _ enrich.RunSource = (*GameModesSource)(nil)