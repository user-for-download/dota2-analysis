package domain

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


