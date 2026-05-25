package dotaconstants

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich"
)

type PatchesSource struct {
	BaseURL string
	Writer  enrich.PatchesWriter
	HTTP    enrich.HTTPClient
}

func NewPatchesSource(baseURL string, w enrich.PatchesWriter, httpClient ...enrich.HTTPClient) *PatchesSource {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	var h enrich.HTTPClient
	if len(httpClient) > 0 {
		h = httpClient[0]
	}
	return &PatchesSource{BaseURL: baseURL, Writer: w, HTTP: h}
}

func (s *PatchesSource) Name() string        { return "dotaconstants:patch" }
func (s *PatchesSource) Critical() bool       { return true }
func (s *PatchesSource) DependsOn() []string  { return nil }

type patchJSON struct {
	ID   int             `json:"id"`
	Name string          `json:"name"`
	Date json.RawMessage `json:"date"`
}

func (s *PatchesSource) Run(ctx context.Context, deps enrich.Deps) error {
	client := deps.HTTP
	if s.HTTP != nil {
		client = s.HTTP
	}
	raw, err := fetchJSON[[]patchJSON](ctx, client, s.BaseURL, "patch.json")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	refs := make([]enrich.PatchRef, 0, len(raw))
	for _, p := range raw {
		if p.Name == "" {
			continue
		}
		refs = append(refs, enrich.PatchRef{
			ID:        p.ID,
			Name:      p.Name,
			ReleaseAt: parsePatchDate(p.Date),
		})
	}
	return s.Writer.UpsertPatches(ctx, refs)
}

func parsePatchDate(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}
	// Try string first (RFC3339 or epoch-as-string).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if epoch, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Unix(epoch, 0).UTC()
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC()
		}
		return time.Time{}
	}
	// Try bare number (epoch seconds) — the actual format in dotaconstants.
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return time.Unix(int64(f), 0).UTC()
	}
	// Try array of any (legacy fallback).
	var arr []any
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		if f, ok := arr[0].(float64); ok {
			return time.Unix(int64(f), 0).UTC()
		}
	}
	return time.Time{}
}

var _ enrich.RunSource = (*PatchesSource)(nil)