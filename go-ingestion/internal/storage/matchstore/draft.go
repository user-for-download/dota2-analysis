package matchstore

// PickBanRow represents a single pick or ban in the draft phase.
type PickBanRow struct {
	Order  int16
	IsPick bool
	HeroID int16
	Team   int16
}

// DraftTimingRow records timing details for a single draft pick.
type DraftTimingRow struct {
	Order          int16
	Pick           bool
	ActiveTeam     int16
	HeroID         int16
	PlayerSlot     int16
	ExtraTime      int32
	TotalTimeTaken int32
}
