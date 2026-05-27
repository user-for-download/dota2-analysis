package enrich

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich/gate"
)

// RunnerIface abstracts the enrichment runner for testability.
type RunnerIface interface {
	Run(ctx context.Context) error
}

type RunnerOptions struct {
	Sources []RunSource
	HTTP    HTTPClient
	Gate    gate.RunGate
	Logger  *slog.Logger
}

type Runner struct {
	sources []RunSource
	http    HTTPClient
	gate    gate.RunGate
	log     *slog.Logger
}

func NewRunner(opts RunnerOptions) (*Runner, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	sorted, skipped, err := sortByDependencies(opts.Sources)
	if err != nil {
		return nil, fmt.Errorf("runner: dependency cycle detected: %w", err)
	}
	if len(skipped) > 0 {
		opts.Logger.Warn("runner: unknown source types skipped", "count", len(skipped), "names", skipped)
	}
	return &Runner{
		sources: sorted,
		http:    opts.HTTP,
		gate:    opts.Gate,
		log:     opts.Logger.With("component", "enrich.runner"),
	}, nil
}

func sortByDependencies(sources []RunSource) (sorted []RunSource, skipped []string, err error) {
	type node struct {
		id   string
		deps []string
		idx  int
	}

	nodes := make([]node, 0, len(sources))
	nameMap := make(map[string]int)
	skipped = make([]string, 0)

	for i, s := range sources {
		name := s.Name()
		deps := s.DependsOn()
		nodes = append(nodes, node{id: name, deps: deps, idx: i})
		nameMap[name] = len(nodes) - 1
	}

	sorted = make([]RunSource, 0, len(nodes))
	inStack := make(map[string]bool)
	done := make(map[string]bool)

	var visit func(name string) error
	visit = func(name string) error {
		if done[name] {
			return nil
		}
		if inStack[name] {
			return fmt.Errorf("cycle detected involving %q", name)
		}

		inStack[name] = true

		idx, known := nameMap[name]
		if known {
			for _, dep := range nodes[idx].deps {
				if _, ok := nameMap[dep]; !ok {
					skipped = append(skipped, dep)
					continue
				}
				if err := visit(dep); err != nil {
					return err
				}
			}
			if !done[name] {
				sorted = append(sorted, sources[nodes[idx].idx])
				done[name] = true
			}
		}

		inStack[name] = false
		return nil
	}

	for _, n := range nodes {
		if err := visit(n.id); err != nil {
			return nil, nil, err
		}
	}

	return sorted, skipped, nil
}

func (r *Runner) Run(ctx context.Context) error {
	if r.http == nil || r.gate == nil {
		return fmt.Errorf("enrich: runner not fully wired (http/gate)")
	}
	for _, s := range r.sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.runOne(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) runOne(ctx context.Context, s RunSource) error {
	name := s.Name()

	log := r.log.With("source", name)

	shouldRun, err := r.gate.ShouldRun(ctx, name)
	if err != nil {
		log.Error("source: gate check failed", "err", err)
		return fmt.Errorf("gate check: %w", err)
	}
	if !shouldRun {
		log.Info("source: skipped by gate")
		return nil
	}

	log.Info("source: starting")
	start := time.Now()

	err = s.Run(ctx, Deps{HTTP: r.http, Logger: r.log})
	if err != nil {
		outcome := gate.RunOutcome{Success: false, Err: err.Error(), At: start}
		_ = r.gate.RecordRun(ctx, name, outcome)
		if s.Critical() {
			log.Error("source: run failed (critical)", "err", err)
			return fmt.Errorf("run: %w", err)
		}
		log.Warn("source: run failed (non-critical, continuing)", "err", err)
		return nil
	}

	outcome := gate.RunOutcome{Success: true, At: start}
	if err := r.gate.RecordRun(ctx, name, outcome); err != nil {
		log.Warn("source: record run failed", "err", err)
	}

	log.Info("source: done", "duration", time.Since(start))
	return nil
}