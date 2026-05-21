package loader

import "context"

type Source interface {
	Name() string
	Load(ctx context.Context) ([]string, error)
}