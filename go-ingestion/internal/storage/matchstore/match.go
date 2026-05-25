package matchstore

// Match is the root domain object for a parsed Dota 2 match.
//
// Scalar fields are organized into embedded sub-structs so field access
// remains flat (m.MatchID, m.Duration, …) while the type system keeps
// related fields grouped visually and in memory.
type Match struct {
	MatchIdentity
	MatchBuildings
	MatchLobby
	MatchContext
	MatchTeams
	MatchReplay
	IsParsed bool

	// ─── Sub-object collections ───────────────────
	Players      []PlayerRow
	Details      []PlayerDetailRow
	PicksBans    []PickBanRow
	DraftTimings []DraftTimingRow
	Objectives   []ObjectiveRow
	Chat         []ChatRow
	Teamfights   []TeamfightRow
	Advantages   *AdvantagesRow
	Cosmetics    []byte
	Timeseries   []TimeseriesRow

	// Raw JSON payload (excluded from serialization).
	Raw []byte `json:"-"`
}
