package api

import (
	"net/http"
	"strconv"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// PlayerHeroDTO represents a player's performance with a specific hero.
type PlayerHeroDTO struct {
	HeroID     int16   `json:"hero_id"`
	HeroName   string  `json:"hero_name"`
	Games      int     `json:"games"`
	Wins       int     `json:"wins"`
	WRShrunk   float64 `json:"wr_shrunk"`
	LastPlayed int64   `json:"last_played"`
}

// PlayerProfileResponse is the full response for GET /v1/players/{account_id}/profile.
type PlayerProfileResponse struct {
	AccountID   int64           `json:"account_id"`
	HeroHistory []PlayerHeroDTO `json:"hero_history"`
	RecentTeams []PlayerTeamDTO `json:"recent_teams"`
}

// PlayerTeamDTO represents a player's recent team affiliations.
type PlayerTeamDTO struct {
	TeamID     int64 `json:"team_id"`
	Games      int   `json:"games"`
	LastPlayed int64 `json:"last_played"`
	IsRecent   bool  `json:"is_recent"`
}

// PlayerProfile handles GET /v1/players/{id}/profile.
func (h *Handler) PlayerProfile(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	accountID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid account id"}`, http.StatusBadRequest)
		return
	}

	// Query player hero history from mv_player_hero_profile
	heroHistory, err := h.repo.PlayerHeroes(r.Context(), domain.AccountID(accountID), 1, 50)
	if err != nil {
		h.log.Error("player hero query", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	dtoHeroHistory := make([]PlayerHeroDTO, len(heroHistory))
	for i, ph := range heroHistory {
		dtoHeroHistory[i] = PlayerHeroDTO{
			HeroID:     int16(ph.HeroID),
			HeroName:   ph.HeroName,
			Games:      ph.Games,
			Wins:       ph.Wins,
			WRShrunk:   ph.WRShrunk,
			LastPlayed: ph.LastPlayed.Unix(),
		}
	}

	// Query player team history from mv_player_team_history
	teams, err := h.repo.PlayerTeams(r.Context(), domain.AccountID(accountID), 10)
	if err != nil {
		h.log.Error("player team query", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	dtoTeams := make([]PlayerTeamDTO, len(teams))
	for i, pt := range teams {
		dtoTeams[i] = PlayerTeamDTO{
			TeamID:     pt.TeamID,
			Games:      pt.Games,
			LastPlayed: pt.LastPlayed.Unix(),
			IsRecent:   pt.LastPatchID > 0,
		}
	}

	resp := PlayerProfileResponse{
		AccountID:   accountID,
		HeroHistory: dtoHeroHistory,
		RecentTeams: dtoTeams,
	}

	h.writeJSON(w, resp)
}
