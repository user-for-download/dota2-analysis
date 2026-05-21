package dotaconstants

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/user-for-download/go-dota2/internal/enrich"
)

type LobbyTypesSource struct {
	BaseURL string
	Writer  enrich.LobbyTypesWriter
	HTTP    enrich.HTTPClient
}

func NewLobbyTypesSource(baseURL string, w enrich.LobbyTypesWriter, httpClient ...enrich.HTTPClient) *LobbyTypesSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	var h enrich.HTTPClient
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &LobbyTypesSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *LobbyTypesSource) Name() string        { return "dotaconstants:lobby_type" }
func (s *LobbyTypesSource) Critical() bool       { return false }
func (s *LobbyTypesSource) DependsOn() []string  { return nil }

type lobbyTypeJSON struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Balanced bool   `json:"balanced"`
}

func (s *LobbyTypesSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	raw, err := fetchJSON[map[string]lobbyTypeJSON](ctx, client, s.BaseURL, "lobby_type.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	refs := make([]enrich.LobbyTypeRef, 0, len(raw))
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
		refs = append(refs, enrich.LobbyTypeRef{ID: id, Name: v.Name, Balanced: v.Balanced})
	}
	return s.Writer.UpsertLobbyTypes(ctx, refs)
}

var _ enrich.RunSource = (*LobbyTypesSource)(nil)