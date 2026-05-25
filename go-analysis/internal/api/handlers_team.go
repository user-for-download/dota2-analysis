package api

import (
	"net/http"
	"strconv"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// TeamProfile handles GET /v1/teams/{id}/profile.
func (h *Handler) TeamProfile(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	teamID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid team id"}`, http.StatusBadRequest)
		return
	}

	history, err := h.repo.TeamHeroes(r.Context(), domain.TeamID(teamID), 1, 50)
	if err != nil {
		h.log.Error("team profile query", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	dtoHistory := make([]TeamHeroHistoryDTO, len(history))
	for i, th := range history {
		dtoHistory[i] = TeamHeroHistoryDTO{
			HeroID:   int16(th.HeroID),
			HeroName: th.HeroName,
			Games:    th.Games,
			Wins:     th.Wins,
			WRShrunk: th.WRShrunk,
		}
	}

	resp := TeamProfileResponse{
		TeamID:      teamID,
		HeroHistory: dtoHistory,
	}

	h.writeJSON(w, resp)
}

// H2H handles GET /v1/h2h?team_a=X&team_b=Y.
func (h *Handler) H2H(w http.ResponseWriter, r *http.Request) {
	teamA, err := strconv.ParseInt(r.URL.Query().Get("team_a"), 10, 64)
	if err != nil || teamA == 0 {
		http.Error(w, `{"error":"team_a and team_b required"}`, http.StatusBadRequest)
		return
	}
	teamB, err := strconv.ParseInt(r.URL.Query().Get("team_b"), 10, 64)
	if err != nil || teamB == 0 {
		http.Error(w, `{"error":"team_a and team_b required"}`, http.StatusBadRequest)
		return
	}

	rec, err := h.repo.TeamH2H(r.Context(), domain.TeamID(teamA), domain.TeamID(teamB))
	if err != nil {
		h.log.Error("h2h query", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	resp := H2HResponse{
		TeamA:     teamA,
		TeamB:     teamB,
		Games:     rec.Games,
		TeamAWins: rec.TeamAWins,
		TeamBWins: rec.TeamBWins,
	}

	h.writeJSON(w, resp)
}
