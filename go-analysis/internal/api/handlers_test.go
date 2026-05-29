package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/config"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/profiles"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/recommend"
)

// ─── Mocks ──────────────────────────────────────────────────

type mockRepository struct {
	heroSynergiesFn  func(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]profiles.HeroPair, error)
	heroCountersFn   func(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]profiles.HeroPair, error)
	teamHeroesFn     func(ctx context.Context, teamID domain.TeamID, minGames, limit int) ([]profiles.TeamHero, error)
	teamH2HFn        func(ctx context.Context, teamA, teamB domain.TeamID) (profiles.H2HRecord, error)
	playerHeroesFn   func(ctx context.Context, accountID domain.AccountID, minGames, limit int) ([]profiles.PlayerHero, error)
	playerTeamsFn    func(ctx context.Context, accountID domain.AccountID, limit int) ([]profiles.PlayerTeam, error)
	featurizerStatusFn func(ctx context.Context) (profiles.FeaturizerStatus, error)
}

func (m *mockRepository) HeroSynergies(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]profiles.HeroPair, error) {
	return m.heroSynergiesFn(ctx, heroID, minGames, limit)
}
func (m *mockRepository) HeroCounters(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]profiles.HeroPair, error) {
	return m.heroCountersFn(ctx, heroID, minGames, limit)
}
func (m *mockRepository) TeamHeroes(ctx context.Context, teamID domain.TeamID, minGames, limit int) ([]profiles.TeamHero, error) {
	return m.teamHeroesFn(ctx, teamID, minGames, limit)
}
func (m *mockRepository) TeamH2H(ctx context.Context, teamA, teamB domain.TeamID) (profiles.H2HRecord, error) {
	return m.teamH2HFn(ctx, teamA, teamB)
}
func (m *mockRepository) PlayerHeroes(ctx context.Context, accountID domain.AccountID, minGames, limit int) ([]profiles.PlayerHero, error) {
	return m.playerHeroesFn(ctx, accountID, minGames, limit)
}
func (m *mockRepository) PlayerTeams(ctx context.Context, accountID domain.AccountID, limit int) ([]profiles.PlayerTeam, error) {
	return m.playerTeamsFn(ctx, accountID, limit)
}
func (m *mockRepository) FeaturizerStatus(ctx context.Context) (profiles.FeaturizerStatus, error) {
	return m.featurizerStatusFn(ctx)
}
func (m *mockRepository) TeamHeroStatsBatch(ctx context.Context, teamID domain.TeamID, heroes []domain.HeroID) (map[domain.HeroID]profiles.TeamHeroStats, error) {
	return nil, nil
}
func (m *mockRepository) SynergyAvgBatch(ctx context.Context, allies []domain.HeroID, candidates []domain.HeroID) (map[domain.HeroID]float64, error) {
	return nil, nil
}
func (m *mockRepository) CounterAvgBatch(ctx context.Context, candidates []domain.HeroID, enemies []domain.HeroID) (map[domain.HeroID]float64, error) {
	return nil, nil
}
func (m *mockRepository) RosterComfortAvgBatch(ctx context.Context, roster []domain.AccountID, heroes []domain.HeroID) (map[domain.HeroID]float64, error) {
	return nil, nil
}
func (m *mockRepository) StarThreatBatch(ctx context.Context, themTeamID domain.TeamID, heroes []domain.HeroID, minGames int) (map[domain.HeroID]float64, error) {
	return nil, nil
}
func (m *mockRepository) GlobalHeroStatsBatch(ctx context.Context, heroes []domain.HeroID, patchID domain.PatchID) (map[domain.HeroID]profiles.GlobalHeroStats, error) {
	return nil, nil
}
func (m *mockRepository) GlobalTotalPicks(ctx context.Context, patchID domain.PatchID) (int, error) {
	return 10000, nil
}

type mockRecommender struct {
	recommendFn func(ctx context.Context, st *domain.DraftState, k int) (*domain.Result, error)
}

func (m *mockRecommender) Recommend(ctx context.Context, st *domain.DraftState, k int) (*domain.Result, error) {
	return m.recommendFn(ctx, st, k)
}

type mockCatalog struct{}

func (mockCatalog) Name(id domain.HeroID) string            { return "npc_dota_hero_axe" }
func (mockCatalog) Info(id domain.HeroID) (domain.HeroInfo, bool) {
	return domain.HeroInfo{ID: id, Name: "npc_dota_hero_axe"}, true
}
func (mockCatalog) Roles(id domain.HeroID) []domain.Role     { return nil }
func (mockCatalog) All() []domain.HeroID                     { return nil }
func (mockCatalog) EachHero(f func(domain.HeroID) bool)      {}

// ─── Helpers ────────────────────────────────────────────────

func newTestHandler(repo profiles.Repository, recommender recommend.Recommender) *Handler {
	return NewHandler(repo, config.AnalyticsConfig{CurrentPatchID: 42}, recommender, mockCatalog{}, nil, slog.Default())
}

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func parseJSON(t *testing.T, body *bytes.Buffer, v any) {
	t.Helper()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

// ─── Tests ──────────────────────────────────────────────────

func TestHealthEndpoint(t *testing.T) {
	now := time.Now()
	repo := &mockRepository{
		featurizerStatusFn: func(ctx context.Context) (profiles.FeaturizerStatus, error) {
			return profiles.FeaturizerStatus{LastSuccessful: now}, nil
		},
	}
	h := newTestHandler(repo, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp HealthResponse
	parseJSON(t, w.Body, &resp)
	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.PatchID != 42 {
		t.Errorf("expected patch_id 42, got %d", resp.PatchID)
	}
	if resp.Scorer != "linear" {
		t.Errorf("expected scorer 'linear', got %q", resp.Scorer)
	}
}

func TestHealthEndpoint_Stale(t *testing.T) {
	old := time.Now().Add(-72 * time.Hour)
	repo := &mockRepository{
		featurizerStatusFn: func(ctx context.Context) (profiles.FeaturizerStatus, error) {
			return profiles.FeaturizerStatus{LastSuccessful: old}, nil
		},
	}
	h := newTestHandler(repo, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)

	var resp HealthResponse
	parseJSON(t, w.Body, &resp)
	if resp.Status != "stale" {
		t.Errorf("expected status 'stale', got %q", resp.Status)
	}
	if resp.FeaturizerStalenessHours == nil {
		t.Fatal("expected staleness_hours, got nil")
	}
	if *resp.FeaturizerStalenessHours < 70 {
		t.Errorf("expected staleness > 70h, got %f", *resp.FeaturizerStalenessHours)
	}
}

func TestHealthEndpoint_RepoError(t *testing.T) {
	repo := &mockRepository{
		featurizerStatusFn: func(ctx context.Context) (profiles.FeaturizerStatus, error) {
			return profiles.FeaturizerStatus{}, nil // zero time
		},
	}
	h := newTestHandler(repo, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)

	var resp HealthResponse
	parseJSON(t, w.Body, &resp)
	if resp.Status != "stale" {
		t.Errorf("expected 'stale' when no data, got %q", resp.Status)
	}
}

func TestHeroSynergy(t *testing.T) {
	repo := &mockRepository{
		heroSynergiesFn: func(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]profiles.HeroPair, error) {
			return []profiles.HeroPair{
				{HeroID: 2, HeroName: "npc_dota_hero_axe", Games: 100, Wins: 60, WRShrunk: 0.58},
			}, nil
		},
	}
	h := newTestHandler(repo, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/heroes/1/synergy", nil)
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.HeroSynergy(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp HeroSynergyResponse
	parseJSON(t, w.Body, &resp)
	if len(resp.Partners) != 1 {
		t.Fatalf("expected 1 partner, got %d", len(resp.Partners))
	}
	if resp.Partners[0].HeroID != 2 {
		t.Errorf("expected hero_id 2, got %d", resp.Partners[0].HeroID)
	}
}

func TestHeroSynergy_BadID(t *testing.T) {
	h := newTestHandler(&mockRepository{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/heroes/abc/synergy", nil)
	r.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	h.HeroSynergy(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHeroCounter(t *testing.T) {
	repo := &mockRepository{
		heroCountersFn: func(ctx context.Context, heroID domain.HeroID, minGames, limit int) ([]profiles.HeroPair, error) {
			return []profiles.HeroPair{
				{HeroID: 3, HeroName: "npc_dota_hero_puck", Games: 50, Wins: 30, WRShrunk: 0.55},
			}, nil
		},
	}
	h := newTestHandler(repo, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/heroes/1/counter", nil)
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.HeroCounter(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp HeroCounterResponse
	parseJSON(t, w.Body, &resp)
	if len(resp.Counters) != 1 {
		t.Fatalf("expected 1 counter, got %d", len(resp.Counters))
	}
}

func TestTeamProfile(t *testing.T) {
	repo := &mockRepository{
		teamHeroesFn: func(ctx context.Context, teamID domain.TeamID, minGames, limit int) ([]profiles.TeamHero, error) {
			return []profiles.TeamHero{
				{HeroID: 1, HeroName: "npc_dota_hero_axe", Games: 50, Wins: 30, WRShrunk: 0.58},
			}, nil
		},
	}
	h := newTestHandler(repo, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/teams/123/profile", nil)
	r.SetPathValue("id", "123")
	w := httptest.NewRecorder()
	h.TeamProfile(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp TeamProfileResponse
	parseJSON(t, w.Body, &resp)
	if resp.TeamID != 123 {
		t.Errorf("expected team_id 123, got %d", resp.TeamID)
	}
}

func TestH2H(t *testing.T) {
	repo := &mockRepository{
		teamH2HFn: func(ctx context.Context, teamA, teamB domain.TeamID) (profiles.H2HRecord, error) {
			return profiles.H2HRecord{Games: 10, TeamAWins: 7, TeamBWins: 3}, nil
		},
	}
	h := newTestHandler(repo, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/h2h?team_a=1&team_b=2", nil)
	w := httptest.NewRecorder()
	h.H2H(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp H2HResponse
	parseJSON(t, w.Body, &resp)
	if resp.Games != 10 || resp.TeamAWins != 7 || resp.TeamBWins != 3 {
		t.Errorf("unexpected H2H: %+v", resp)
	}
}

func TestH2H_MissingParams(t *testing.T) {
	h := newTestHandler(&mockRepository{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/h2h", nil)
	w := httptest.NewRecorder()
	h.H2H(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPlayerProfile(t *testing.T) {
	repo := &mockRepository{
		playerHeroesFn: func(ctx context.Context, accountID domain.AccountID, minGames, limit int) ([]profiles.PlayerHero, error) {
			return []profiles.PlayerHero{
				{HeroID: 1, HeroName: "npc_dota_hero_axe", Games: 100, Wins: 55, WRShrunk: 0.54, LastPlayed: time.Now()},
			}, nil
		},
		playerTeamsFn: func(ctx context.Context, accountID domain.AccountID, limit int) ([]profiles.PlayerTeam, error) {
			return []profiles.PlayerTeam{
				{TeamID: 10, Games: 20, Wins: 12, LastPlayed: time.Now(), LastPatchID: 42},
			}, nil
		},
	}
	h := newTestHandler(repo, nil)

	r := httptest.NewRequest(http.MethodGet, "/v1/players/12345/profile", nil)
	r.SetPathValue("id", "12345")
	w := httptest.NewRecorder()
	h.PlayerProfile(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp PlayerProfileResponse
	parseJSON(t, w.Body, &resp)
	if resp.AccountID != 12345 {
		t.Errorf("expected account_id 12345, got %d", resp.AccountID)
	}
	if len(resp.HeroHistory) != 1 {
		t.Errorf("expected 1 hero, got %d", len(resp.HeroHistory))
	}
	if len(resp.RecentTeams) != 1 {
		t.Errorf("expected 1 team, got %d", len(resp.RecentTeams))
	}
}

func TestRecommendEndpoint(t *testing.T) {
	repo := &mockRepository{
		featurizerStatusFn: func(ctx context.Context) (profiles.FeaturizerStatus, error) {
			return profiles.FeaturizerStatus{LastSuccessful: time.Now()}, nil
		},
	}
	rec := &mockRecommender{
		recommendFn: func(ctx context.Context, st *domain.DraftState, k int) (*domain.Result, error) {
			return &domain.Result{
				Recommendations: []domain.Recommendation{
					{Hero: 1, Name: "npc_dota_hero_axe", Score: 0.95, Rank: 1},
				},
			}, nil
		},
	}
	h := newTestHandler(repo, rec)

	body := RecommendRequest{
		PatchID:       42,
		UserTeam:      "radiant",
		RadiantTeamID: 100,
		DireTeamID:    200,
		Slot:         1, // 1-based; maps to 0-based slot 0 = first Radiant ban
		K:            3,
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/recommend", jsonBody(t, body))
	w := httptest.NewRecorder()
	h.Recommend(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	var resp RecommendResponse
	parseJSON(t, w.Body, &resp)
	if len(resp.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(resp.Recommendations))
	}
	if resp.Recommendations[0].Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", resp.Recommendations[0].Score)
	}
}

func TestRecommend_BadBody(t *testing.T) {
	h := newTestHandler(&mockRepository{}, nil)

	r := httptest.NewRequest(http.MethodPost, "/v1/recommend", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	h.Recommend(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	handler := AuthMiddleware("test-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	handler := AuthMiddleware("test-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	r.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	handler := AuthMiddleware("test-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	r.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_WrongFormat(t *testing.T) {
	handler := AuthMiddleware("test-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	r.Header.Set("Authorization", "Basic test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
