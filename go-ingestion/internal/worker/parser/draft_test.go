package parser

import (
	"testing"

	"github.com/user-for-download/dota2-analysis/go-core/domain"
)

// cmPB is a shorthand for building a PickBan.
func cmPB(isPick bool, heroID int, team int16, order int) domain.PickBan {
	return domain.PickBan{
		IsPick: isPick,
		HeroID: domain.HeroID(heroID),
		Team:   team,
		Order:  int16(order),
	}
}

// build7_33Plus creates a complete 24-slot 7.33+ draft with the given hero IDs.
func build7_33Plus(heroIDs ...int) []domain.PickBan {
	if len(heroIDs) == 0 {
		heroIDs = make([]int, 24)
		for i := range heroIDs {
			heroIDs[i] = i + 1
		}
	}
	pbs := make([]domain.PickBan, 24)
	team := int16(0)
	// 7 bans for team 0 (Radiant)
	for i := 0; i < 7; i++ {
		idx := i
		if i >= 4 { // ban 5-7 alternate
			team = int16((i - 4) % 2)
		} else {
			team = int16(i % 2)
		}
		pbs[idx] = cmPB(false, heroIDs[idx], team, idx+1)
	}
	// 2 picks (Radiant, Dire)
	pbs[7] = cmPB(true, heroIDs[7], 0, 8)
	pbs[8] = cmPB(true, heroIDs[8], 1, 9)
	// 3 bans (Radiant, Dire, Radiant)
	pbs[9] = cmPB(false, heroIDs[9], 0, 10)
	pbs[10] = cmPB(false, heroIDs[10], 1, 11)
	pbs[11] = cmPB(false, heroIDs[11], 0, 12)
	// 6 picks alternating
	for i := 0; i < 6; i++ {
		t := int16(i % 2)
		pbs[12+i] = cmPB(true, heroIDs[12+i], t, 13+i)
	}
	// 4 bans alternating
	for i := 0; i < 4; i++ {
		t := int16(i % 2)
		pbs[18+i] = cmPB(false, heroIDs[18+i], t, 19+i)
	}
	// 2 picks
	pbs[22] = cmPB(true, heroIDs[22], 0, 23)
	pbs[23] = cmPB(true, heroIDs[23], 1, 24)
	return pbs
}

// build7_33PlusDeclinedBans returns a 7.33+ draft with 3 bans declined
// (removed bans at positions that would be team 0, team 1, team 0).
func build7_33PlusDeclinedBans() []domain.PickBan {
	pbs := build7_33Plus()
	// Remove bans at indices 1, 3, and 10 (all bans — false entries)
	// Keep picks intact. This simulates declined bans.
	out := make([]domain.PickBan, 0, len(pbs)-3)
	for i, pb := range pbs {
		if i == 1 || i == 3 || i == 10 {
			continue // declined ban — skip it
		}
		out = append(out, pb)
	}
	// Re-number orders
	for i := range out {
		out[i].Order = int16(i + 1)
	}
	return out
}

// buildPre730 creates a complete 20-slot pre-7.30 draft.
// Phase structure: 4 bans, 6 picks, 6 bans, 4 picks.
func buildPre730() []domain.PickBan {
	pbs := make([]domain.PickBan, 20)
	// 4 bans (2R, 2D)
	pbs[0] = cmPB(false, 1, 0, 1)
	pbs[1] = cmPB(false, 2, 0, 2)
	pbs[2] = cmPB(false, 3, 1, 3)
	pbs[3] = cmPB(false, 4, 1, 4)
	// 6 picks (R,D,R,D,R,D)
	for i := 0; i < 6; i++ {
		t := int16(i % 2)
		pbs[4+i] = cmPB(true, int(5+i), t, 5+i)
	}
	// 6 bans (R,D,R,D,R,D)
	for i := 0; i < 6; i++ {
		t := int16(i % 2)
		pbs[10+i] = cmPB(false, int(11+i), t, 11+i)
	}
	// 4 picks (R,D,R,D)
	for i := 0; i < 4; i++ {
		t := int16(i % 2)
		pbs[16+i] = cmPB(true, int(17+i), t, 17+i)
	}
	return pbs
}

// build730To732 creates a complete 24-slot 7.30-7.32 draft.
func build730To732() []domain.PickBan {
	pbs := make([]domain.PickBan, 24)
	// 4 bans (2R, 2D)
	pbs[0] = cmPB(false, 1, 0, 1)
	pbs[1] = cmPB(false, 2, 0, 2)
	pbs[2] = cmPB(false, 3, 1, 3)
	pbs[3] = cmPB(false, 4, 1, 4)
	// 4 picks (R, D, R, D)
	for i := 0; i < 4; i++ {
		t := int16(i % 2)
		pbs[4+i] = cmPB(true, int(5+i), t, 5+i)
	}
	// 6 bans (R, D, R, D, R, D)
	for i := 0; i < 6; i++ {
		t := int16(i % 2)
		pbs[8+i] = cmPB(false, int(9+i), t, 9+i)
	}
	// 4 picks (R, D, R, D)
	for i := 0; i < 4; i++ {
		t := int16(i % 2)
		pbs[14+i] = cmPB(true, int(15+i), t, 15+i)
	}
	// 4 bans (R, D, R, D)
	for i := 0; i < 4; i++ {
		t := int16(i % 2)
		pbs[18+i] = cmPB(false, int(19+i), t, 19+i)
	}
	// 2 picks (R, D)
	pbs[22] = cmPB(true, 23, 0, 23)
	pbs[23] = cmPB(true, 24, 1, 24)
	return pbs
}

func TestValidateDraft_Valid(t *testing.T) {
	cases := []struct {
		name string
		pbs  []domain.PickBan
	}{
		{"7.33+ perfect", build7_33Plus()},
		{"7.33+ declined bans", build7_33PlusDeclinedBans()},
		{"pre-7.30", buildPre730()},
		{"7.30-7.32", build730To732()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateDraft(c.pbs); err != nil {
				t.Errorf("ValidateDraft() = %v, want nil", err)
			}
		})
	}
}

func TestValidateDraft_Invalid(t *testing.T) {
	t.Run("too few entries", func(t *testing.T) {
		pbs := make([]domain.PickBan, 10)
		for i := range pbs {
			pbs[i] = cmPB(true, i+1, 0, i+1)
		}
		if err := ValidateDraft(pbs); err == nil {
			t.Error("ValidateDraft() = nil, want error (length 10)")
		}
	})

	t.Run("too many entries", func(t *testing.T) {
		pbs := make([]domain.PickBan, 30)
		for i := range pbs {
			pbs[i] = cmPB(i%2 == 0, i+1, int16(i%2), i+1)
		}
		if err := ValidateDraft(pbs); err == nil {
			t.Error("ValidateDraft() = nil, want error (length 30)")
		}
	})

	t.Run("wrong pick count (12 picks)", func(t *testing.T) {
		pbs := build7_33Plus()
		// Replace a ban with a pick (changes pick count to 11)
		pbs[0] = cmPB(true, 100, 0, 1)
		if err := ValidateDraft(pbs); err == nil {
			t.Error("ValidateDraft() = nil, want error (11 picks)")
		}
	})

	t.Run("wrong pick count (8 picks)", func(t *testing.T) {
		pbs := build7_33Plus()
		// Replace two picks with bans
		pbs[7] = cmPB(false, 100, 0, 8)
		pbs[8] = cmPB(false, 101, 1, 9)
		if err := ValidateDraft(pbs); err == nil {
			t.Error("ValidateDraft() = nil, want error (8 picks)")
		}
	})

	t.Run("nonsense B/P sequence", func(t *testing.T) {
		// Alternating pick/ban every slot — doesn't match any era
		pbs := make([]domain.PickBan, 20)
		for i := range pbs {
			pbs[i] = cmPB(i%2 == 0, i+1, int16(i%2), i+1)
		}
		if err := ValidateDraft(pbs); err == nil {
			t.Error("ValidateDraft() = nil, want error (nonsense sequence)")
		}
	})

	t.Run("removed pick instead of ban", func(t *testing.T) {
		pbs := build7_33Plus()
		// Remove a pick (not allowed — only bans can be declined)
		out := make([]domain.PickBan, 0, len(pbs)-1)
		for i, pb := range pbs {
			if i == 7 { // skip first pick
				continue
			}
			out = append(out, pb)
		}
		for i := range out {
			out[i].Order = int16(i + 1)
		}
		if err := ValidateDraft(out); err == nil {
			t.Error("ValidateDraft() = nil, want error (removed pick)")
		}
	})
}

func TestDetectDraftEra(t *testing.T) {
	cases := []struct {
		name string
		pbs  []domain.PickBan
		want string
	}{
		{"7.33+ perfect", build7_33Plus(), "7_33_plus"},
		{"7.33+ declined bans", build7_33PlusDeclinedBans(), "7_33_plus"},
		{"pre-7.30", buildPre730(), "pre_7_30"},
		{"7.30-7.32", build730To732(), "7_30_to_7_32"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DetectDraftEra(c.pbs)
			if got != c.want {
				t.Errorf("DetectDraftEra() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDetectDraftEra_Unknown(t *testing.T) {
	// Nonsense sequence should return "unknown"
	pbs := make([]domain.PickBan, 20)
	for i := range pbs {
		pbs[i] = cmPB(i%2 == 0, i+1, int16(i%2), i+1)
	}
	if got := DetectDraftEra(pbs); got != "unknown" {
		t.Errorf("DetectDraftEra() = %q, want %q", got, "unknown")
	}
}

func TestAssignPhases(t *testing.T) {
	t.Run("7.33+ full draft", func(t *testing.T) {
		pbs := build7_33Plus()
		phases := AssignPhases(pbs)
		// Phase layout: 7 bans(0-6), 2 picks(7-8), 3 bans(9-11),
		//               6 picks(12-17), 4 bans(18-21), 2 picks(22-23)
		expected := []int{
			0, 0, 0, 0, 0, 0, 0, // 7 bans
			1, 1, // 2 picks
			2, 2, 2, // 3 bans
			3, 3, 3, 3, 3, 3, // 6 picks
			4, 4, 4, 4, // 4 bans
			5, 5, // 2 picks
		}
		if len(phases) != len(expected) {
			t.Fatalf("got %d phases, want %d", len(phases), len(expected))
		}
		for i := range expected {
			if phases[i] != expected[i] {
				t.Errorf("phase[%d] = %d, want %d", i, phases[i], expected[i])
			}
		}
	})

	t.Run("7.33+ with declined bans (stable phase numbers)", func(t *testing.T) {
		pbs := build7_33PlusDeclinedBans()
		phases := AssignPhases(pbs)
		// Same phase numbers despite missing bans
		// removed: idx 1 (phase 0), idx 3 (phase 0), idx 10 (phase 2)
		expected := []int{
			0, 0, 0, 0, 0, // 5 remaining bans (was 7)
			1, 1, // 2 picks
			2, 2, // 2 remaining bans (was 3)
			3, 3, 3, 3, 3, 3, // 6 picks
			4, 4, 4, 4, // 4 bans
			5, 5, // 2 picks
		}
		if len(phases) != len(expected) {
			t.Fatalf("got %d phases, want %d", len(phases), len(expected))
		}
		for i := range expected {
			if phases[i] != expected[i] {
				t.Errorf("phase[%d] = %d, want %d", i, phases[i], expected[i])
			}
		}
	})
}

func TestPhaseName(t *testing.T) {
	cases := []struct {
		phase  int
		isPick bool
		want   string
	}{
		{0, false, "ban_1"},
		{1, true, "pick_1"},
		{2, false, "ban_2"},
		{3, true, "pick_2"},
		{4, false, "ban_3"},
		{5, true, "pick_3"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			got := PhaseName(c.phase, c.isPick)
			if got != c.want {
				t.Errorf("PhaseName(%d, %v) = %q, want %q", c.phase, c.isPick, got, c.want)
			}
		})
	}
}

func TestIsSubsequenceRemovesOnlyBans(t *testing.T) {
	pattern := []bool{false, false, true, true, false, true, true, false}

	t.Run("exact match", func(t *testing.T) {
		observed := []bool{false, false, true, true, false, true, true, false}
		if !isSubsequenceRemovesOnlyBans(observed, pattern) {
			t.Error("exact match should pass")
		}
	})

	t.Run("declined one ban", func(t *testing.T) {
		observed := []bool{false, true, true, false, true, true, false}
		if !isSubsequenceRemovesOnlyBans(observed, pattern) {
			t.Error("removing one ban (first false) should pass")
		}
	})

	t.Run("declined two bans", func(t *testing.T) {
		observed := []bool{false, true, true, true, true, false}
		if !isSubsequenceRemovesOnlyBans(observed, pattern) {
			t.Error("removing two bans should pass")
		}
	})

	t.Run("removed a pick — should fail", func(t *testing.T) {
		observed := []bool{false, false, true, false, true, true, false}
		if isSubsequenceRemovesOnlyBans(observed, pattern) {
			t.Error("removing a pick should fail")
		}
	})

	t.Run("empty observed", func(t *testing.T) {
		var observed []bool
		if isSubsequenceRemovesOnlyBans(observed, pattern) {
			t.Error("empty observed should fail")
		}
	})
}
