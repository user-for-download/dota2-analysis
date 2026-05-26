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
var cmPhaseTableData = []Phase{
	{Name: "ban_1", IsBan: true, ActingTeam: DraftRadiant},    // 0
	{Name: "ban_1", IsBan: true, ActingTeam: DraftDire},       // 1
	{Name: "ban_2", IsBan: true, ActingTeam: DraftRadiant},    // 2
	{Name: "ban_2", IsBan: true, ActingTeam: DraftDire},       // 3
	{Name: "pick_1", IsBan: false, ActingTeam: DraftDire},     // 4
	{Name: "pick_1", IsBan: false, ActingTeam: DraftRadiant},  // 5
	{Name: "pick_2", IsBan: false, ActingTeam: DraftRadiant},  // 6
	{Name: "ban_3", IsBan: true, ActingTeam: DraftRadiant},    // 7
	{Name: "ban_3", IsBan: true, ActingTeam: DraftDire},       // 8
	{Name: "pick_2", IsBan: false, ActingTeam: DraftDire},     // 9
	{Name: "pick_3", IsBan: false, ActingTeam: DraftRadiant},  // 10
	{Name: "ban_4", IsBan: true, ActingTeam: DraftDire},       // 11
	{Name: "ban_4", IsBan: true, ActingTeam: DraftRadiant},    // 12
	{Name: "pick_4", IsBan: false, ActingTeam: DraftRadiant},  // 13
	{Name: "pick_3", IsBan: false, ActingTeam: DraftDire},     // 14
	{Name: "pick_4", IsBan: false, ActingTeam: DraftDire},     // 15
	{Name: "pick_5", IsBan: false, ActingTeam: DraftRadiant},  // 16
	{Name: "ban_5", IsBan: true, ActingTeam: DraftDire},       // 17
	{Name: "ban_5", IsBan: true, ActingTeam: DraftRadiant},    // 18
	{Name: "pick_5", IsBan: false, ActingTeam: DraftDire},     // 19
	{Name: "ban_6", IsBan: true, ActingTeam: DraftRadiant},    // 20
	{Name: "ban_6", IsBan: true, ActingTeam: DraftDire},       // 21
	{Name: "ban_7", IsBan: true, ActingTeam: DraftRadiant},    // 22
	{Name: "ban_7", IsBan: true, ActingTeam: DraftDire},       // 23
}

// cmTable is the singleton default phase table.
var cmTable PhaseTable = &phaseTable{phases: cmPhaseTableData}

// CMPhaseTable returns the standard Captain's Mode draft phase table.
func CMPhaseTable() PhaseTable { return cmTable }


