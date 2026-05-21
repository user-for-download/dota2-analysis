package api

// RecommendRequest is the JSON body for POST /v1/recommend.
type RecommendRequest struct {
	PatchID       int32   `json:"patch_id"`
	UserTeam      string  `json:"user_team"`       // "radiant" or "dire"
	RadiantTeamID int64   `json:"radiant_team_id"`
	DireTeamID    int64   `json:"dire_team_id"`
	RadiantRoster []int64 `json:"radiant_roster"` // account IDs
	DireRoster    []int64 `json:"dire_roster"`
	RadiantPicks  []int16 `json:"radiant_picks"`
	DirePicks     []int16 `json:"dire_picks"`
	RadiantBans   []int16 `json:"radiant_bans"`
	DireBans      []int16 `json:"dire_bans"`
	Slot          int     `json:"slot"` // 1-based; converted to 0-based internally
	K             int     `json:"k"`    // top-K recommendations
}

// RecommendResponse is the JSON response for POST /v1/recommend.
type RecommendResponse struct {
	Phase           string              `json:"phase"`
	IsBan           bool                `json:"is_ban"`
	ActingTeam      string              `json:"acting_team"`
	UsedValueModel  bool                `json:"used_value_model"`
	Warnings        []string            `json:"warnings,omitempty"`
	Recommendations []RecommendationDTO `json:"recommendations"`
}

// RecommendationDTO is the JSON-serializable form of domain.Recommendation.
type RecommendationDTO struct {
	HeroID  int16    `json:"hero_id"`
	Name    string   `json:"name"`
	Score   float64  `json:"score"`
	Rank    int      `json:"rank"`
	Reasons []string `json:"reasons,omitempty"`
	Risks   []string `json:"risks,omitempty"`
}

// DraftSimulateRequest is the JSON body for POST /v1/draft/simulate.
type DraftSimulateRequest struct {
	PatchID       int32   `json:"patch_id"`
	RadiantTeamID int64   `json:"radiant_team_id"`
	DireTeamID    int64   `json:"dire_team_id"`
	RadiantRoster []int64 `json:"radiant_roster"`
	DireRoster    []int64 `json:"dire_roster"`
	K             int     `json:"k"`
}

// DraftSimulateResponse is the JSON response for POST /v1/draft/simulate.
type DraftSimulateResponse struct {
	Steps []DraftStepDTO `json:"steps"`
}

// DraftStepDTO represents one step in a simulated draft.
type DraftStepDTO struct {
	Slot            int                 `json:"slot"`
	Phase           string              `json:"phase"`
	IsBan           bool                `json:"is_ban"`
	ActingTeam      string              `json:"acting_team"`
	UsedValueModel  bool                `json:"used_value_model"`
	Recommendations []RecommendationDTO `json:"recommendations"`
}

// TeamProfileResponse is the JSON response for GET /v1/teams/{id}/profile.
type TeamProfileResponse struct {
	TeamID      int64                `json:"team_id"`
	Name        string               `json:"name"`
	HeroHistory []TeamHeroHistoryDTO `json:"hero_history"`
}

// TeamHeroHistoryDTO represents a team's history with a specific hero.
type TeamHeroHistoryDTO struct {
	HeroID   int16   `json:"hero_id"`
	HeroName string  `json:"hero_name"`
	Games    int     `json:"games"`
	Wins     int     `json:"wins"`
	WRShrunk float64 `json:"wr_shrunk"`
}

// H2HResponse is the JSON response for GET /v1/h2h.
type H2HResponse struct {
	TeamA     int64 `json:"team_a"`
	TeamB     int64 `json:"team_b"`
	Games     int   `json:"games"`
	TeamAWins int   `json:"team_a_wins"`
	TeamBWins int   `json:"team_b_wins"`
}

// HeroSynergyResponse is the JSON response for GET /v1/heroes/{id}/synergy.
type HeroSynergyResponse struct {
	HeroID   int16               `json:"hero_id"`
	HeroName string              `json:"hero_name"`
	Partners []SynergyPartnerDTO `json:"partners"`
}

// SynergyPartnerDTO represents a hero's synergy with another hero.
type SynergyPartnerDTO struct {
	HeroID   int16   `json:"hero_id"`
	HeroName string  `json:"hero_name"`
	Games    int     `json:"games"`
	Wins     int     `json:"wins"`
	WRShrunk float64 `json:"wr_shrunk"`
}

// HeroCounterResponse is the JSON response for GET /v1/heroes/{id}/counter.
type HeroCounterResponse struct {
	HeroID   int16              `json:"hero_id"`
	HeroName string             `json:"hero_name"`
	Counters []CounterPickDTO   `json:"counters"`
}

// CounterPickDTO represents a hero that counters another hero.
type CounterPickDTO struct {
	HeroID   int16   `json:"hero_id"`
	HeroName string  `json:"hero_name"`
	Games    int     `json:"games"`
	Wins     int     `json:"wins"`
	WRShrunk float64 `json:"wr_shrunk"`
}

