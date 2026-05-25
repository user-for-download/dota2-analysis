package matchstore

// ObjectiveRow represents a single in-game objective (tower, roshan, etc.).
type ObjectiveRow struct {
	Time       int32
	Type       string
	Slot       int16
	PlayerSlot int16
	Team       int16
	Key        string
	Value      int32
	Unit       string
	Raw        []byte
}

// ChatRow represents a single chat message from the match.
type ChatRow struct {
	Time       int32
	Type       string
	PlayerSlot int16
	Unit       string
	Key        string
}

// TeamfightRow summarizes a single teamfight.
type TeamfightRow struct {
	EndTime   int32
	LastDeath int32
	Deaths    int16
	Players   []byte
}

// AdvantagesRow holds radiant gold/xp advantage over time.
type AdvantagesRow struct {
	RadiantGoldAdv []int32
	RadiantXPAdv   []int32
}

// TimeseriesRow holds per-player per-minute stats.
type TimeseriesRow struct {
	PlayerSlot int16
	Minute     int16
	HeroID     int16
	AccountID  int64
	PatchID    int32
	Gold       int32
	XP         int32
	LH         int16
	DN         int16
}
