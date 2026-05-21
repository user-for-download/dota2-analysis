package bootstrap

import "context"

type BootstrapMarker interface {
	Done(ctx context.Context, sourceName string) (bool, error)
	MarkDone(ctx context.Context, sourceName string) error
}