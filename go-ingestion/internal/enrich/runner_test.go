package enrich

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/enrich/gate"
)

// ---------------------------------------------------------------------------
// Shared test doubles (kept minimal — only what each test needs)
// ---------------------------------------------------------------------------

type fakeHTTP struct{}

func (fakeHTTP) Get(context.Context, string) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(emptyReader{})}, nil
}

type emptyReader struct{}

func (emptyReader) Read(p []byte) (int, error) { return 0, io.EOF }

type fakeGate struct {
	shouldRun bool
	recordErr error
}

func (g *fakeGate) ShouldRun(context.Context, string) (bool, error) { return g.shouldRun, nil }
func (g *fakeGate) RecordRun(context.Context, string, gate.RunOutcome) error {
	return g.recordErr
}

// stubSource implements the legacy Source interface (no DependsOn).
type stubSource struct {
	name     string
	critical bool
	fetchErr error
	applyErr error
	fetched  bool
	applied  bool
}

func (s *stubSource) Name() string   { return s.name }
func (s *stubSource) Critical() bool { return s.critical }
func (s *stubSource) DependsOn() []string { return nil }
func (s *stubSource) Run(ctx context.Context, deps Deps) error {
	data, err := s.Fetch(ctx, deps.HTTP)
	if err != nil {
		return err
	}
	return s.Apply(ctx, data)
}
func (s *stubSource) Fetch(context.Context, HTTPClient) (any, error) {
	s.fetched = true
	return struct{}{}, s.fetchErr
}
func (s *stubSource) Apply(context.Context, any) error {
	s.applied = true
	return s.applyErr
}

// depSource implements RunSource so it can declare dependencies.
type depSource struct {
	name     string
	critical bool
	deps     []string
	runErr   error
	ran      bool
}

func (s *depSource) Name() string        { return s.name }
func (s *depSource) Critical() bool      { return s.critical }
func (s *depSource) DependsOn() []string { return s.deps }
func (s *depSource) Run(_ context.Context, _ Deps) error {
	s.ran = true
	return s.runErr
}

func newRunner(t *testing.T, srcs []RunSource, g *fakeGate) *Runner {
	t.Helper()
	if g == nil {
		g = &fakeGate{shouldRun: true}
	}
	r, err := NewRunner(RunnerOptions{
		Sources: srcs,
		HTTP:    fakeHTTP{},
		Gate:    g,
		Logger:  slog.Default(),
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	return r
}

// ---------------------------------------------------------------------------
// sortByDependencies — unit tests
// ---------------------------------------------------------------------------

// namesOf returns the names of sorted sources in order, for easy assertion.
func namesOf(sorted []RunSource) []string {
	out := make([]string, 0, len(sorted))
	for _, s := range sorted {
		out = append(out, s.Name())
	}
	return out
}

func TestSortNoDeps(t *testing.T) {
	// No dependencies at all — order should be preserved.
	a := &depSource{name: "a"}
	b := &depSource{name: "b"}
	c := &depSource{name: "c"}

	sorted, skipped, err := sortByDependencies([]RunSource{a, b, c})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none", skipped)
	}
	if got := namesOf(sorted); strings.Join(got, ",") != "a,b,c" {
		t.Errorf("order = %v, want [a b c]", got)
	}
}

func TestSortLinearChain(t *testing.T) {
	// c → b → a  (c depends on b, b depends on a)
	// Expected output order: a, b, c
	a := &depSource{name: "a", deps: nil}
	b := &depSource{name: "b", deps: []string{"a"}}
	c := &depSource{name: "c", deps: []string{"b"}}

	// Deliberately register in reverse order to prove sort works.
	sorted, _, err := sortByDependencies([]RunSource{c, b, a})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := namesOf(sorted); strings.Join(got, ",") != "a,b,c" {
		t.Errorf("order = %v, want [a b c]", got)
	}
}

// TestSortSharedDependency is the exact scenario that triggered the original
// bug. Both b and c depend on a. The old code used a single visited map for
// both "in-stack" and "done" detection, so the second top-level visit that
// tried to recurse into a would see visited["a"]==true and return
// "cycle detected" even though the graph is a valid DAG.
func TestSortSharedDependency(t *testing.T) {
	//   a
	//  / \
	// b   c    (b and c both depend on a)
	a := &depSource{name: "a"}
	b := &depSource{name: "b", deps: []string{"a"}}
	c := &depSource{name: "c", deps: []string{"a"}}

	sorted, _, err := sortByDependencies([]RunSource{a, b, c})
	if err != nil {
		t.Fatalf("shared dependency falsely reported as cycle: %v", err)
	}

	names := namesOf(sorted)

	// a must appear before both b and c.
	idxA := indexOf(names, "a")
	idxB := indexOf(names, "b")
	idxC := indexOf(names, "c")

	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("sorted = %v, missing expected names", names)
	}
	if idxA >= idxB {
		t.Errorf("a must come before b: %v", names)
	}
	if idxA >= idxC {
		t.Errorf("a must come before c: %v", names)
	}
}

// TestSortSharedDependencyNoDuplicate verifies that a node reachable via
// multiple paths appears exactly once in the output (the old code would
// append it multiple times if the false-cycle error didn't fire first).
func TestSortSharedDependencyNoDuplicate(t *testing.T) {
	a := &depSource{name: "a"}
	b := &depSource{name: "b", deps: []string{"a"}}
	c := &depSource{name: "c", deps: []string{"a"}}

	sorted, _, err := sortByDependencies([]RunSource{a, b, c})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 3 {
		t.Errorf("len(sorted) = %d, want 3 (no duplicates); got %v", len(sorted), namesOf(sorted))
	}
}

func TestSortDiamondDAG(t *testing.T) {
	// Classic diamond: d→b, d→c, b→a, c→a
	//     a
	//    / \
	//   b   c
	//    \ /
	//     d
	a := &depSource{name: "a"}
	b := &depSource{name: "b", deps: []string{"a"}}
	c := &depSource{name: "c", deps: []string{"a"}}
	d := &depSource{name: "d", deps: []string{"b", "c"}}

	sorted, _, err := sortByDependencies([]RunSource{d, c, b, a})
	if err != nil {
		t.Fatalf("diamond DAG falsely reported as cycle: %v", err)
	}

	names := namesOf(sorted)
	if len(names) != 4 {
		t.Fatalf("len = %d, want 4 (no duplicates); got %v", len(names), names)
	}

	mustBefore(t, names, "a", "b")
	mustBefore(t, names, "a", "c")
	mustBefore(t, names, "b", "d")
	mustBefore(t, names, "c", "d")
}

func TestSortDirectCycleDetected(t *testing.T) {
	// a → b → a
	a := &depSource{name: "a", deps: []string{"b"}}
	b := &depSource{name: "b", deps: []string{"a"}}

	_, _, err := sortByDependencies([]RunSource{a, b})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error %q should mention cycle", err)
	}
}

func TestSortSelfCycleDetected(t *testing.T) {
	a := &depSource{name: "a", deps: []string{"a"}}

	_, _, err := sortByDependencies([]RunSource{a})
	if err == nil {
		t.Fatal("expected cycle error for self-dependency, got nil")
	}
}

func TestSortLongerCycleDetected(t *testing.T) {
	// a → b → c → a
	a := &depSource{name: "a", deps: []string{"b"}}
	b := &depSource{name: "b", deps: []string{"c"}}
	c := &depSource{name: "c", deps: []string{"a"}}

	_, _, err := sortByDependencies([]RunSource{a, b, c})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestSortFailsOnUnknownDependency(t *testing.T) {
	a := &depSource{name: "a", deps: []string{"does-not-exist"}}

	_, _, err := sortByDependencies([]RunSource{a})
	if err == nil {
		t.Fatal("expected error for unknown dependency, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error %q should mention the missing dependency", err)
	}
}

/*
func TestSortSkipsUnknownTypes(t *testing.T) {
	a := &depSource{name: "a"}
	unknown := "i am not a source"

	sorted, skipped, err := sortByDependencies([]RunSource{a, unknown})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 1 {
		t.Errorf("sorted len = %d, want 1", len(sorted))
	}
	if len(skipped) != 1 {
		t.Errorf("skipped len = %d, want 1", len(skipped))
	}
}
*/

func TestSortMixedSourceAndRunSource(t *testing.T) {
	// Legacy Source mixed with RunSource. RunSource b depends on legacy a.
	// Legacy Source has no DependsOn, so it can't declare deps, but it can
	// be depended upon.
	a := &stubSource{name: "a"}                       // legacy Source
	b := &depSource{name: "b", deps: []string{"a"}} // RunSource

	sorted, _, err := sortByDependencies([]RunSource{b, a})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mustBefore(t, namesOf(sorted), "a", "b")
}

// ---------------------------------------------------------------------------
// Runner — integration tests (existing behaviour, now also covers RunSource)
// ---------------------------------------------------------------------------

func TestRunnerHappyPath(t *testing.T) {
	a := &stubSource{name: "a", critical: true}
	b := &stubSource{name: "b", critical: false}
	r := newRunner(t, []RunSource{a, b}, &fakeGate{shouldRun: true})

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !a.fetched || !a.applied {
		t.Error("source a not fully invoked")
	}
	if !b.fetched || !b.applied {
		t.Error("source b not fully invoked")
	}
}

func TestRunnerCriticalFetchAborts(t *testing.T) {
	a := &stubSource{name: "a", critical: true, fetchErr: errors.New("boom")}
	b := &stubSource{name: "b", critical: true}
	r := newRunner(t, []RunSource{a, b}, nil)

	if err := r.Run(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if b.fetched {
		t.Error("source b ran despite earlier critical failure")
	}
}

func TestRunnerNonCriticalFailureContinues(t *testing.T) {
	a := &stubSource{name: "a", critical: false, fetchErr: errors.New("boom")}
	b := &stubSource{name: "b", critical: true}
	r := newRunner(t, []RunSource{a, b}, nil)

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("non-critical failure should not propagate: %v", err)
	}
	if !b.fetched {
		t.Error("source b should still run")
	}
}

func TestRunnerCriticalApplyAborts(t *testing.T) {
	a := &stubSource{name: "a", critical: true, applyErr: errors.New("apply fail")}
	b := &stubSource{name: "b", critical: true}
	r := newRunner(t, []RunSource{a, b}, nil)

	if err := r.Run(context.Background()); err == nil {
		t.Fatal("expected error from critical apply failure")
	}
	if b.fetched {
		t.Error("source b should not run after critical apply failure")
	}
}

func TestRunnerDependencyOrderEnforced(t *testing.T) {
	// Register in reverse dependency order; runner must execute a before b.
	var executionOrder []string

	a := &orderedSource{name: "a", deps: nil, order: &executionOrder}
	b := &orderedSource{name: "b", deps: []string{"a"}, order: &executionOrder}

	r := newRunner(t, []RunSource{b, a}, nil) // intentionally b before a
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(executionOrder) != 2 {
		t.Fatalf("execution order = %v, want 2 entries", executionOrder)
	}
	if executionOrder[0] != "a" || executionOrder[1] != "b" {
		t.Errorf("execution order = %v, want [a b]", executionOrder)
	}
}

func TestRunnerSharedDependencyRunsOnce(t *testing.T) {
	// b and c both depend on a. a must run exactly once.
	runCount := make(map[string]int)

	a := &countedSource{name: "a", deps: nil, counts: runCount}
	b := &countedSource{name: "b", deps: []string{"a"}, counts: runCount}
	c := &countedSource{name: "c", deps: []string{"a"}, counts: runCount}

	r := newRunner(t, []RunSource{a, b, c}, nil)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if runCount["a"] != 1 {
		t.Errorf("source a ran %d times, want exactly 1", runCount["a"])
	}
}

func TestRunnerCycleRejectedAtConstruction(t *testing.T) {
	a := &depSource{name: "a", deps: []string{"b"}}
	b := &depSource{name: "b", deps: []string{"a"}}

	_, err := NewRunner(RunnerOptions{
		Sources: []RunSource{a, b},
		HTTP:    fakeHTTP{},
		Gate:    &fakeGate{shouldRun: true},
		Logger:  slog.Default(),
	})
	if err == nil {
		t.Fatal("expected NewRunner to reject a cycle, got nil")
	}
}

// ---------------------------------------------------------------------------
// Additional test doubles for ordering/counting tests
// ---------------------------------------------------------------------------

type orderedSource struct {
	name  string
	deps  []string
	order *[]string
}

func (s *orderedSource) Name() string        { return s.name }
func (s *orderedSource) Critical() bool      { return true }
func (s *orderedSource) DependsOn() []string { return s.deps }
func (s *orderedSource) Run(_ context.Context, _ Deps) error {
	*s.order = append(*s.order, s.name)
	return nil
}

type countedSource struct {
	name   string
	deps   []string
	counts map[string]int
}

func (s *countedSource) Name() string        { return s.name }
func (s *countedSource) Critical() bool      { return true }
func (s *countedSource) DependsOn() []string { return s.deps }
func (s *countedSource) Run(_ context.Context, _ Deps) error {
	s.counts[s.name]++
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func indexOf(names []string, target string) int {
	for i, n := range names {
		if n == target {
			return i
		}
	}
	return -1
}

func mustBefore(t *testing.T, names []string, earlier, later string) {
	t.Helper()
	a := indexOf(names, earlier)
	b := indexOf(names, later)
	if a < 0 {
		t.Errorf("%q not found in %v", earlier, names)
		return
	}
	if b < 0 {
		t.Errorf("%q not found in %v", later, names)
		return
	}
	if a >= b {
		t.Errorf("want %q before %q, got order %v", earlier, later, names)
	}
}