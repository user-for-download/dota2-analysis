package matchstore

// MatchLobby describes the lobby configuration and environment.
type MatchLobby struct {
	FirstBloodTime int32
	LobbyType      int16
	GameMode       int16
	Cluster        int16
	Region         int16
	Skill          int16
	Engine         int16
	HumanPlayers   int16
	Version        int16
}

// MatchContext holds patch, voting, league, and series metadata.
type MatchContext struct {
	PatchID       int32
	PositiveVotes int32
	NegativeVotes int32
	LeagueID      int32
	SeriesID      int32
	SeriesType    int16
}
