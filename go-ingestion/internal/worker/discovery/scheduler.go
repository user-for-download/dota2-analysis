package discovery

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Scheduler struct {
	cycles []Cycle
	log    *slog.Logger
}

func NewScheduler(cycles []Cycle, log *slog.Logger) *Scheduler {
	return &Scheduler{cycles: cycles, log: log}
}

func (s *Scheduler) Run(ctx context.Context) {
	for _, c := range s.cycles {
		cycle := c
		interval := cycle.Interval()
		runAtStart := cycle.RunAtStart()

		if runAtStart {
			s.runCycle(ctx, cycle)
		}

		if interval <= 0 {
			continue
		}

		s.log.Info("scheduling cycle",
			"name", cycle.Name(),
			"interval", interval,
		)

		go func() {
			ticker := time.NewTicker(jitter(interval))
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					s.log.Info("cycle exiting", "name", cycle.Name())
					return
				case <-ticker.C:
					s.runCycle(ctx, cycle)
				}
			}
		}()
	}

	<-ctx.Done()
}

func (s *Scheduler) runCycle(ctx context.Context, c Cycle) {
	tracer := otel.Tracer("discoverer")

	cycleCtx, span := tracer.Start(ctx, "cycle.run", trace.WithAttributes(
		attribute.String("cycle.name", c.Name()),
	))
	defer span.End()

	s.log.Info("cycle starting", "name", c.Name())
	if err := c.RunOnce(cycleCtx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.log.Error("cycle failed", "name", c.Name(), "err", err)
	} else {
		span.SetStatus(codes.Ok, "success")
		s.log.Info("cycle done", "name", c.Name())
	}
}

func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	maxJitter := d / 10
	if maxJitter <= 0 {
		return d
	}
	offset := time.Duration(rand.Int63n(int64(maxJitter)))
	return d + offset
}