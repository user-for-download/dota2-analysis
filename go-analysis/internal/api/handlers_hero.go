package api

import (
	"net/http"
	"strconv"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// HeroSynergy handles GET /v1/heroes/{id}/synergy.
func (h *Handler) HeroSynergy(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	heroID, err := strconv.ParseInt(idStr, 10, 16)
	if err != nil {
		http.Error(w, `{"error":"invalid hero id"}`, http.StatusBadRequest)
		return
	}

	partners, err := h.repo.HeroSynergies(r.Context(), domain.HeroID(heroID), 2, 20)
	if err != nil {
		h.log.Error("synergy query", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	dtoPartners := make([]SynergyPartnerDTO, len(partners))
	for i, p := range partners {
		dtoPartners[i] = SynergyPartnerDTO{
			HeroID:   int16(p.HeroID),
			HeroName: p.HeroName,
			Games:    p.Games,
			Wins:     p.Wins,
			WRShrunk: p.WRShrunk,
		}
	}

	resp := HeroSynergyResponse{
		HeroID:   int16(heroID),
		HeroName: h.catalog.Name(domain.HeroID(heroID)),
		Partners: dtoPartners,
	}

	h.writeJSON(w, resp)
}

// HeroCounter handles GET /v1/heroes/{id}/counter.
func (h *Handler) HeroCounter(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	heroID, err := strconv.ParseInt(idStr, 10, 16)
	if err != nil {
		http.Error(w, `{"error":"invalid hero id"}`, http.StatusBadRequest)
		return
	}

	counters, err := h.repo.HeroCounters(r.Context(), domain.HeroID(heroID), 2, 20)
	if err != nil {
		h.log.Error("counter query", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	dtoCounters := make([]CounterPickDTO, len(counters))
	for i, c := range counters {
		dtoCounters[i] = CounterPickDTO{
			HeroID:   int16(c.HeroID),
			HeroName: c.HeroName,
			Games:    c.Games,
			Wins:     c.Wins,
			WRShrunk: c.WRShrunk,
		}
	}

	resp := HeroCounterResponse{
		HeroID:   int16(heroID),
		HeroName: h.catalog.Name(domain.HeroID(heroID)),
		Counters: dtoCounters,
	}

	h.writeJSON(w, resp)
}
