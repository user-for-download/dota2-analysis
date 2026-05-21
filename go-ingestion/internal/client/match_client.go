package client

import (
	"context"
)

type MatchClient interface {
	GetMatch(ctx context.Context, matchID int64) ([]byte, error)
}