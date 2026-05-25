package api

import (
	"encoding/json"
	"net/http"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// Recommend handles POST /v1/recommend.
func (h *Handler) Recommend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RecommendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.K <= 0 {
		req.K = 10
	}

	// Convert request to DraftState
	userTeam := domain.SideUs
	if req.UserTeam == "dire" {
		userTeam = domain.SideThem
	}

	// Convert roster
	radiantRoster := make([]domain.AccountID, len(req.RadiantRoster))
	for i, id := range req.RadiantRoster {
		radiantRoster[i] = domain.AccountID(id)
	}
	direRoster := make([]domain.AccountID, len(req.DireRoster))
	for i, id := range req.DireRoster {
		direRoster[i] = domain.AccountID(id)
	}

	// Convert picks/bans
	radiantPicks := make([]domain.HeroID, len(req.RadiantPicks))
	for i, id := range req.RadiantPicks {
		radiantPicks[i] = domain.HeroID(id)
	}
	direPicks := make([]domain.HeroID, len(req.DirePicks))
	for i, id := range req.DirePicks {
		direPicks[i] = domain.HeroID(id)
	}
	radiantBans := make([]domain.HeroID, len(req.RadiantBans))
	for i, id := range req.RadiantBans {
		radiantBans[i] = domain.HeroID(id)
	}
	direBans := make([]domain.HeroID, len(req.DireBans))
	for i, id := range req.DireBans {
		direBans[i] = domain.HeroID(id)
	}

	st := domain.NewDraftState(
		domain.PatchID(req.PatchID),
		userTeam,
		domain.CMPhaseTable(),
		domain.TeamID(req.RadiantTeamID), domain.TeamID(req.DireTeamID),
		radiantRoster, direRoster,
		radiantPicks, direPicks,
		radiantBans, direBans,
		req.Slot-1, // DTO is 1-based, convert to 0-based internal index
	)

	result, err := h.recommender.Recommend(r.Context(), st, req.K)
	if err != nil {
		h.log.Error("recommend", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	phase := st.Phase()
	resp := RecommendResponse{
		Phase:          phase.Name,
		IsBan:          phase.IsBan,
		ActingTeam:     phase.ActingTeam.String(),
		UsedValueModel: result.UsedValueModel,
		Warnings:       result.Warnings,
	}
	for _, rec := range result.Recommendations {
		dto := RecommendationDTO{
			HeroID: int16(rec.Hero),
			Name:   rec.Name,
			Score:  rec.Score,
			Rank:   rec.Rank,
		}
		for _, r := range rec.Reasons {
			dto.Reasons = append(dto.Reasons, r.Factor+": "+r.Note)
		}
		for _, r := range rec.Risks {
			dto.Risks = append(dto.Risks, r)
		}
		resp.Recommendations = append(resp.Recommendations, dto)
	}

	h.writeJSON(w, resp)
}

// SimulateDraft handles POST /v1/draft/simulate.
func (h *Handler) SimulateDraft(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DraftSimulateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.K <= 0 {
		req.K = 5
	}

	// Simulate the full draft, getting recommendations at each pick slot
	phases := domain.CMPhaseTable()
	resp := DraftSimulateResponse{Steps: make([]DraftStepDTO, 0, phases.Len())}

	radiantRoster := make([]domain.AccountID, len(req.RadiantRoster))
	for i, id := range req.RadiantRoster {
		radiantRoster[i] = domain.AccountID(id)
	}
	direRoster := make([]domain.AccountID, len(req.DireRoster))
	for i, id := range req.DireRoster {
		direRoster[i] = domain.AccountID(id)
	}

	var radiantPicks, direPicks, radiantBans, direBans []domain.HeroID

	for slot := 0; slot < phases.Len(); slot++ {
		phase, _ := phases.At(slot)
		userTeam := domain.SideUs
		if phase.ActingTeam == domain.DraftDire {
			userTeam = domain.SideThem
		}
		st := domain.NewDraftState(
			domain.PatchID(req.PatchID),
			userTeam,
			domain.CMPhaseTable(),
			domain.TeamID(req.RadiantTeamID), domain.TeamID(req.DireTeamID),
			radiantRoster, direRoster,
			radiantPicks, direPicks,
			radiantBans, direBans,
			slot,
		)

		result, err := h.recommender.Recommend(r.Context(), st, req.K)
		if err != nil {
			h.log.Error("simulate step", "slot", slot, "err", err)
			continue
		}

		phase = st.Phase()
		step := DraftStepDTO{
			Slot:           slot + 1, // convert back to 1-based for API response
			Phase:         phase.Name,
			IsBan:         phase.IsBan,
			ActingTeam:    phase.ActingTeam.String(),
			UsedValueModel: result.UsedValueModel,
		}
		for _, rec := range result.Recommendations {
			dto := RecommendationDTO{
				HeroID: int16(rec.Hero),
				Name:   rec.Name,
				Score:  rec.Score,
				Rank:   rec.Rank,
			}
			step.Recommendations = append(step.Recommendations, dto)
		}
		resp.Steps = append(resp.Steps, step)

		// Simulate picking the top recommendation (for picks only)
		if !phase.IsBan && len(result.Recommendations) > 0 {
			topHero := result.Recommendations[0].Hero
			if phase.ActingTeam == domain.DraftRadiant {
				radiantPicks = append(radiantPicks, topHero)
			} else {
				direPicks = append(direPicks, topHero)
			}
		}
	}

	h.writeJSON(w, resp)
}
