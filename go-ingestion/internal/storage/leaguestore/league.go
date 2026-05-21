package leaguestore

import "context"

type League struct {
	LeagueID int64
	Name     string
	Tier     string
	Ticket   string
	Banner   string
}

type Writer interface {
	Upsert(ctx context.Context, leagues []League) (int, error)
}