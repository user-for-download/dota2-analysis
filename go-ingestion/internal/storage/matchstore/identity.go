package matchstore

// MatchIdentity holds the primary key, timing, and outcome of a match.
type MatchIdentity struct {
	MatchID     int64
	MatchSeqNum int64
	StartTime   int64
	Duration    int32
	RadiantWin  bool
}

// MatchBuildings tracks tower/barracks status and team scores.
type MatchBuildings struct {
	TowerStatusRadiant    int16
	TowerStatusDire       int16
	BarracksStatusRadiant int16
	BarracksStatusDire    int16
	RadiantScore          int16
	DireScore             int16
}
