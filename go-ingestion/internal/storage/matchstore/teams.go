package matchstore

// MatchTeams identifies the participating teams and their captains.
type MatchTeams struct {
	RadiantTeamID  int64
	DireTeamID     int64
	RadiantCaptain int64
	DireCaptain    int64
}

// MatchReplay holds replay file metadata.
type MatchReplay struct {
	ReplaySalt int64
	ReplayURL  string
	Pauses     []byte
}
