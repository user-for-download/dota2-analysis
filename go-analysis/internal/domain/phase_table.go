package domain

import "fmt"

// PhaseTable defines the ordered list of draft phases.
// Implementations are immutable; the same table is shared across all DraftState instances.
type PhaseTable interface {
	At(slot int) (Phase, bool)
	Len() int
}

// phaseTable is a concrete implementation backed by a slice.
type phaseTable struct{ phases []Phase }

func (t *phaseTable) At(slot int) (Phase, bool) {
	if slot < 0 || slot >= len(t.phases) {
		return Phase{}, false
	}
	return t.phases[slot], true
}

func (t *phaseTable) Len() int { return len(t.phases) }

// cmPhaseTableData is the standard Captain's Mode draft order (24 slots).
// Updated to match the modern Dota 2 CM draft phase (post-7.33+).
var cmPhaseTableData = []Phase{
	{Name: "ban_1", IsBan: true, ActingTeam: DraftRadiant},    // 0: Ban R
	{Name: "ban_1", IsBan: true, ActingTeam: DraftRadiant},    // 1: Ban R
	{Name: "ban_2", IsBan: true, ActingTeam: DraftDire},       // 2: Ban D
	{Name: "ban_2", IsBan: true, ActingTeam: DraftDire},       // 3: Ban D
	{Name: "ban_3", IsBan: true, ActingTeam: DraftRadiant},    // 4: Ban R
	{Name: "ban_3", IsBan: true, ActingTeam: DraftDire},       // 5: Ban D
	{Name: "ban_4", IsBan: true, ActingTeam: DraftDire},       // 6: Ban D
	{Name: "pick_1", IsBan: false, ActingTeam: DraftRadiant},  // 7: Pick R
	{Name: "pick_1", IsBan: false, ActingTeam: DraftDire},     // 8: Pick D
	{Name: "ban_5", IsBan: true, ActingTeam: DraftRadiant},    // 9: Ban R
	{Name: "ban_5", IsBan: true, ActingTeam: DraftRadiant},    // 10: Ban R
	{Name: "ban_6", IsBan: true, ActingTeam: DraftDire},       // 11: Ban D
	{Name: "pick_2", IsBan: false, ActingTeam: DraftDire},     // 12: Pick D
	{Name: "pick_2", IsBan: false, ActingTeam: DraftRadiant},  // 13: Pick R
	{Name: "pick_3", IsBan: false, ActingTeam: DraftRadiant},  // 14: Pick R
	{Name: "pick_3", IsBan: false, ActingTeam: DraftDire},     // 15: Pick D
	{Name: "pick_4", IsBan: false, ActingTeam: DraftDire},     // 16: Pick D
	{Name: "pick_4", IsBan: false, ActingTeam: DraftRadiant},  // 17: Pick R
	{Name: "ban_7", IsBan: true, ActingTeam: DraftRadiant},    // 18: Ban R
	{Name: "ban_7", IsBan: true, ActingTeam: DraftDire},       // 19: Ban D
	{Name: "ban_8", IsBan: true, ActingTeam: DraftRadiant},    // 20: Ban R
	{Name: "ban_8", IsBan: true, ActingTeam: DraftDire},       // 21: Ban D
	{Name: "pick_5", IsBan: false, ActingTeam: DraftRadiant},  // 22: Pick R
	{Name: "pick_5", IsBan: false, ActingTeam: DraftDire},     // 23: Pick D
}

// cmTable is the singleton default phase table.
var cmTable PhaseTable = &phaseTable{phases: cmPhaseTableData}

// CMPhaseTable returns the standard Captain's Mode draft phase table.
func CMPhaseTable() PhaseTable { return cmTable }

// DerivePhaseTable builds a PhaseTable dynamically from the is_pick sequence.
// Phases are numbered by B/P transitions: each time the sequence switches
// between picks and bans, the phase increments. This gives stable phase
// numbers regardless of declined bans.
//
// Phase names follow the convention "ban_N" / "pick_N" where N increments
// every two phases (since a ban phase and a pick phase form a cycle).
// ActingTeam alternates: ban phases with even index are Radiant, odd are Dire;
// pick phases with even index are Dire, odd are Radiant (matching the
// standard Radiant-first pick alternation in the current CM format).
func DerivePhaseTable(isPick []bool) PhaseTable {
	n := len(isPick)
	if n == 0 {
		return &phaseTable{phases: nil}
	}

	phases := make([]Phase, n)

	// If the draft starts with a pick (the initial ban phases were declined),
	// offset phase 0's current index to 1 so that phase naming stays
	// mathematically aligned: the first pick becomes "pick_1" and the
	// subsequent ban phase becomes "ban_1" instead of both being "ban_1".
	current := 0
	if isPick[0] {
		current = 1
	}

	phases[0] = Phase{
		Name:       phaseName(current, !isPick[0]),
		IsBan:      !isPick[0],
		ActingTeam: actingTeam(current, !isPick[0]),
	}
	for i := 1; i < n; i++ {
		if isPick[i] != isPick[i-1] {
			current++
		}
		phases[i] = Phase{
			Name:       phaseName(current, !isPick[i]),
			IsBan:      !isPick[i],
			ActingTeam: actingTeam(current, !isPick[i]),
		}
	}

	return &phaseTable{phases: phases}
}

// phaseName returns a human-readable name like "ban_1" or "pick_2".
func phaseName(phase int, isBan bool) string {
	pickBan := "ban"
	if !isBan {
		pickBan = "pick"
	}
	return fmt.Sprintf("%s_%d", pickBan, phase/2+1)
}

// actingTeam assigns the acting team based on phase index and type.
// Pattern: within each pair of phases (ban+pick), ban phases alternate
// R→D→R..., pick phases alternate D→R→D... (mirroring the Radiant-first
// convention in the current 7.33+ format).
func actingTeam(phase int, isBan bool) DraftTeam {
	if isBan {
		if phase%2 == 0 {
			return DraftRadiant
		}
		return DraftDire
	}
	// Pick phases: even = Dire, odd = Radiant
	if phase%2 == 0 {
		return DraftDire
	}
	return DraftRadiant
}


