package teamstore

import "context"

type Team struct {
	TeamID        int64
	Name         string
	Tag          string
	LogoURL      string
	Rating      *float64
	Wins        *int
	Losses      *int
	LastMatchTime *int64
	Delta       *float64
	MatchID     *int64
}

type Writer interface {
	Upsert(ctx context.Context, teams []Team) (int, error)
}