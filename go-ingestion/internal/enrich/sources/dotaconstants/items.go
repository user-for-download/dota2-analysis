package dotaconstants

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/user-for-download/go-dota2/internal/enrich"
)

type ItemsSource struct {
	BaseURL string
	Writer  enrich.ItemsWriter
	HTTP    enrich.HTTPClient
}

func NewItemsSource(baseURL string, w enrich.ItemsWriter, httpClient ...enrich.HTTPClient) *ItemsSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	var h enrich.HTTPClient
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &ItemsSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *ItemsSource) Name() string        { return "dotaconstants:items" }
func (s *ItemsSource) Critical() bool       { return true }
func (s *ItemsSource) DependsOn() []string  { return nil }

type itemJSON struct {
	ID       int             `json:"id"`
	DName    string          `json:"dname"`
	Cost     int             `json:"cost"`
	Qual     string          `json:"qual"`
	Behavior json.RawMessage `json:"behavior"`
	Lore     string          `json:"lore"`
	Img      string          `json:"img"`
	Created  bool            `json:"created"`
	Charges  json.RawMessage `json:"charges"`
	CD       json.RawMessage `json:"cd"`
	MC       json.RawMessage `json:"mc"`
}

func (s *ItemsSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	raw, err := fetchJSON[map[string]itemJSON](ctx, client, s.BaseURL, "items.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	refs := make([]enrich.ItemRef, 0, len(raw))
	for key, it := range raw {
		if key == "" || it.ID <= 0 {
			continue
		}
		refs = append(refs, enrich.ItemRef{
			ID:        it.ID,
			Key:       key,
			DName:     it.DName,
			Cost:      it.Cost,
			Qual:      it.Qual,
			Behavior:  rawToString(it.Behavior),
			Lore:      it.Lore,
			Img:       it.Img,
			Created:   it.Created,
			Cooldown:  rawToString(it.CD),
			ManaCost:  rawToString(it.MC),
		})
	}
	return s.Writer.UpsertItems(ctx, refs)
}

var _ enrich.RunSource = (*ItemsSource)(nil)