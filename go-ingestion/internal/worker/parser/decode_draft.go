package parser

import (
	"fmt"
	"slices"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
)

// cmDraftPatterns defines the valid Captain's Mode pick/ban phase structures.
// Each entry is the sequence of is_pick bools for a known draft format.
// These patterns allow for declined bans (removed false entries) during matching.
var cmDraftPatterns = map[string][]bool{
	// Era 1: Pre-7.30 (20 total: 10B + 10P)
	"pre_7_30": {
		false, false,     // 2 bans
		false, false,     // 2 bans
		true, true,       // 2 picks
		true, true,       // 2 picks
		true, true,       // 2 picks
		false, false,     // 2 bans
		false, false,     // 2 bans
		false, false,     // 2 bans
		true, true,       // 2 picks
		true, true,       // 2 picks
	},
	// Era 2: 7.30–7.32 (24 total: 14B + 10P)
	"7_30_to_7_32": {
		false, false, false, false,          // 4 bans
		true, true, true, true,              // 4 picks
		false, false, false, false, false, false, // 6 bans
		true, true, true, true,              // 4 picks
		false, false, false, false,          // 4 bans
		true, true,                          // 2 picks
	},
	// Era 3: 7.33+ (24 total: 14B + 10P, new phase order)
	"7_33_plus": {
		false, false, false, false, false, false, false, // 7 bans
		true, true,                                      // 2 picks
		false, false, false,                             // 3 bans
		true, true, true, true, true, true,              // 6 picks
		false, false, false, false,                      // 4 bans
		true, true,                                      // 2 picks
	},
}

// ValidateDraft validates the draft structure against known CM patterns.
// It allows for declined bans (fewer bans than the pattern specifies)
// as long as the overall structure is consistent.
func ValidateDraft(picksBans []domain.PickBan) error {
	n := len(picksBans)
	if n < 16 || n > 24 {
		return fmt.Errorf("invalid draft length: %d (must be 16–24)", n)
	}

	// Count picks — must always be exactly 10 (5 per team) for CM
	pickCount := 0
	for _, pb := range picksBans {
		if pb.IsPick {
			pickCount++
		}
	}
	if pickCount != 10 {
		return fmt.Errorf("invalid pick count: %d (expected 10)", pickCount)
	}

	// Extract is_pick sequence for pattern matching
	isPickSeq := make([]bool, len(picksBans))
	for i, pb := range picksBans {
		isPickSeq[i] = pb.IsPick
	}

	if !matchesAnyPatternAllowingDeclinedBans(isPickSeq) {
		return fmt.Errorf("draft is_pick sequence does not match any known CM pattern")
	}

	return nil
}

// matchesAnyPatternAllowingDeclinedBans checks if the observed is_pick
// sequence can be derived from a known CM pattern by removing some ban
// entries. This handles teams declining ban phases.
func matchesAnyPatternAllowingDeclinedBans(observed []bool) bool {
	for _, pattern := range cmDraftPatterns {
		if isSubsequenceRemovesOnlyBans(observed, pattern) {
			return true
		}
	}
	return false
}

// isSubsequenceRemovesOnlyBans checks whether observed can be obtained
// from pattern by removing some false (ban) entries only.
// All pick (true) entries must appear in the same relative order and position.
func isSubsequenceRemovesOnlyBans(observed, pattern []bool) bool {
	oi := 0
	for pi := range pattern {
		if oi >= len(observed) {
			break
		}
		if observed[oi] == pattern[pi] {
			oi++
		} else {
			// observed[oi] != pattern[pi]
			// If pattern[pi] is a ban (false), we can skip it (declined ban)
			if !pattern[pi] {
				continue // skip this ban in the pattern
			}
			// If pattern[pi] is a pick (true), we can't skip it
			return false
		}
	}
	return oi == len(observed)
}

// AssignPhases assigns a phase number to each pick/ban entry based on
// B/P transitions. Every time the sequence switches between picks and
// bans, a new phase begins.
//
// For a complete 7.33+ draft (6 phases):
//   Phase 0: 7 bans   (indices 0-6)
//   Phase 1: 2 picks  (indices 7-8)
//   Phase 2: 3 bans   (indices 9-11)
//   Phase 3: 6 picks  (indices 12-17)
//   Phase 4: 4 bans   (indices 18-21)
//   Phase 5: 2 picks  (indices 22-23)
//
// For an incomplete draft (one ban declined in phase 4):
//   Same phase numbers — phase 4 still has 3 bans instead of 4.
func AssignPhases(picksBans []domain.PickBan) []int {
	phases := make([]int, len(picksBans))
	if len(picksBans) == 0 {
		return phases
	}

	currentPhase := 0
	phases[0] = currentPhase

	for i := 1; i < len(picksBans); i++ {
		if picksBans[i].IsPick != picksBans[i-1].IsPick {
			currentPhase++
		}
		phases[i] = currentPhase
	}

	return phases
}

// PhaseName returns a human-readable name for a phase number given whether
// it's a pick phase. Pairs of contiguous ban phases map to the same name;
// pairs of contiguous pick phases map to the same name.
//
// Phase 0 (ban)  → "ban_1"
// Phase 1 (pick) → "pick_1"
// Phase 2 (ban)  → "ban_2"
// Phase 3 (pick) → "pick_2"
// ...
func PhaseName(phase int, isPick bool) string {
	pickBan := "ban"
	if isPick {
		pickBan = "pick"
	}
	return fmt.Sprintf("%s_%d", pickBan, phase/2+1)
}

// PhaseNames returns a human-readable phase name for each entry.
func PhaseNames(picksBans []domain.PickBan) []string {
	phases := AssignPhases(picksBans)
	names := make([]string, len(picksBans))
	for i, phase := range phases {
		names[i] = PhaseName(phase, picksBans[i].IsPick)
	}
	return names
}

// DetectDraftEra returns the era identifier for a validated draft,
// or "unknown" if it doesn't match any known pattern.
func DetectDraftEra(picksBans []domain.PickBan) string {
	isPickSeq := make([]bool, len(picksBans))
	for i, pb := range picksBans {
		isPickSeq[i] = pb.IsPick
	}

	for name, pattern := range cmDraftPatterns {
		if slices.Equal(isPickSeq, pattern) {
			return name
		}
	}
	return "unknown"
}

func decodePicksBans(raw []rawPickBan) []domain.PickBan {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]domain.PickBan, 0, len(raw))
	for _, pb := range raw {
		rows = append(rows, domain.PickBan{
			Order:  pb.Order,
			IsPick: pb.IsPick,
			HeroID: domain.HeroID(pb.HeroID),
			Team:   pb.Team,
		})
	}
	return rows
}

func decodeDraftTimings(raw []rawDraftTiming) []domain.DraftTiming {
	if len(raw) == 0 {
		return nil
	}
	rows := make([]domain.DraftTiming, 0, len(raw))
	for _, d := range raw {
		rows = append(rows, domain.DraftTiming{
			Order:          d.Order,
			Pick:           d.Pick,
			ActiveTeam:     deref16(d.ActiveTeam),
			HeroID:         domain.HeroID(deref16(d.HeroID)),
			PlayerSlot:     deref16(d.PlayerSlot),
			ExtraTime:      deref32(d.ExtraTime),
			TotalTimeTaken: deref32(d.TotalTimeTaken),
		})
	}
	return rows
}
