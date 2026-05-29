package matches

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/dedup"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/metrics"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/storage/matchstore"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/discovery"
	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/worker/fetcher"
)

// localRng is a private per-package random source, avoiding the global
// math/rand mutex that serialises all callers across the entire process.
var localRng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))

type Config struct {
	ExplorerURL string
	Queries     map[string]string
	DefaultKey  string
	Interval    time.Duration
	RunAtStart  bool
	MaxRetries  int
	RetryBackoff time.Duration
	Logger      *slog.Logger
	Dedup       dedup.Seen
	FileKey     string
	Doer        discovery.HTTPDoer
	Reader      matchstore.MatchReader
}

type Cycle struct {
	out    queue.Publisher
	doer   discovery.HTTPDoer
	m      metrics.Sink
	dedup  dedup.Seen
	cfg    Config
	log    *slog.Logger
	reader matchstore.MatchReader
}

func New(out queue.Publisher, doer discovery.HTTPDoer, m metrics.Sink, cfg Config) (*Cycle, error) {
	if out == nil {
		return nil, fmt.Errorf("matches: out queue required")
	}
	if doer == nil {
		return nil, fmt.Errorf("matches: doer required")
	}
	if len(cfg.Queries) == 0 {
		return nil, fmt.Errorf("matches: no queries loaded")
	}
	if cfg.DefaultKey == "" {
		cfg.DefaultKey = "default"
	}
	if _, ok := cfg.Queries[cfg.DefaultKey]; !ok {
		return nil, fmt.Errorf("matches: default query %q not found", cfg.DefaultKey)
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 4
	}
	if cfg.RetryBackoff <= 0 {
		cfg.RetryBackoff = 500 * time.Millisecond
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Cycle{
		out:    out,
		doer:   doer,
		m:      m,
		dedup:  cfg.Dedup,
		cfg:    cfg,
		log:    log.With("component", "discovery.matches"),
		reader: cfg.Reader,
	}, nil
}

func (c *Cycle) Name() string          { return "matches" }
func (c *Cycle) Interval() time.Duration { return c.cfg.Interval }
func (c *Cycle) RunAtStart() bool        { return c.cfg.RunAtStart }

func (c *Cycle) RunOnce(ctx context.Context) error {
	key := c.cfg.DefaultKey
	if c.cfg.FileKey != "" {
		if _, ok := c.cfg.Queries[c.cfg.FileKey]; !ok {
			return fmt.Errorf("query %q not found", c.cfg.FileKey)
		}
		key = c.cfg.FileKey
	}
	sql, ok := c.cfg.Queries[key]
	if !ok {
		return fmt.Errorf("query %q not found", key)
	}

	var refs []matchstore.MatchRef
	var err error
	const maxBackoff = 30 * time.Second
	for attempt := 1; ctx.Err() == nil; attempt++ {
		c.log.Debug("discoverer: fetching match ids", "key", key, "attempt", attempt)
		refs, err = c.fetchMatchRefs(ctx, sql)
		if err == nil {
			break
		}
		c.log.Warn("fetch match ids failed, retrying",
			"key", key, "attempt", attempt, "err", err,
		)
		backoff := c.cfg.RetryBackoff * time.Duration(attempt)
		if backoff > maxBackoff || backoff <= 0 {
			backoff = maxBackoff
		}
		// Add random jitter of ±50% to prevent thundering herd when the
		// upstream API rate-limits or goes down — all discovery workers
		// will fail at the same time and retry simultaneously without jitter.
		jitter := time.Duration(localRng.Int64N(int64(backoff)))
		backoff = backoff/2 + jitter/2
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	if err != nil {
		return fmt.Errorf("fetch match ids (%s): %w", key, err)
	}
	c.log.Info("query returned", "key", key, "count", len(refs))

	// Pre-filter with PostgreSQL to respect existing parsed matches even if Redis resets.
	// Passing (match_id, start_time) pairs enables partition pruning on matches
	// (partitioned by start_time) — without start_time, PG probes every quarterly
	// partition, causing massive CPU/IO spikes at 10k+ candidates.
	c.log.Debug("discoverer: filtering against db", "candidates", len(refs))
	if c.reader != nil && len(refs) > 0 {
		unknownIDs, err := c.reader.UnknownIDs(ctx, refs)
		if err != nil {
			c.log.Warn("failed to check unknown ids against db", "err", err)
		} else {
			c.log.Info("filtered discovered matches against db", "original", len(refs), "unknown", len(unknownIDs))
			// Rebuild refs from the IDs returned — UnknownIDs return value is
			// just match_ids (the subset that need processing). We re-use
			// whichever refs matched those IDs to preserve start_time later.
			idSet := make(map[int64]struct{}, len(unknownIDs))
			for _, id := range unknownIDs {
				idSet[id] = struct{}{}
			}
			filtered := make([]matchstore.MatchRef, 0, len(unknownIDs))
			for _, ref := range refs {
				if _, ok := idSet[ref.MatchID]; ok {
					filtered = append(filtered, ref)
				}
			}
			refs = filtered
		}
	}

	pushed := 0
	skipped := 0
	for _, ref := range refs {
		id := ref.MatchID
		c.log.Debug("discoverer: processing match id", "match_id", id)
		if c.dedup != nil {
			dedupKey := strconv.FormatInt(id, 10)
			seen, err := c.dedup.IsSeen(ctx, dedupKey)
			if err != nil {
				c.log.Warn("dedup check failed", "match_id", id, "err", err)
			} else if seen {
				skipped++
				continue
			}
		}
		payload, err := json.Marshal(fetcher.Task{MatchID: id})
		if err != nil {
			c.log.Warn("marshal task", "match_id", id, "err", err)
			continue
		}
		if err := c.out.Publish(ctx, queue.Message{Payload: payload}); err != nil {
			return fmt.Errorf("queue publish failed at match_id %d: %w", id, err)
		}
		pushed++
	}
	c.log.Info("pushed tasks", "key", key, "pushed", pushed, "skipped", skipped, "discovered", len(refs))
	return nil
}

func (c *Cycle) fetchMatchRefs(ctx context.Context, sql string) ([]matchstore.MatchRef, error) {
	base := c.cfg.ExplorerURL
	if i := strings.Index(base, "?sql="); i > 0 {
		base = base[:i]
	}
	u := base + "?sql=" + url.QueryEscape(sql)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go-dota2/discoverer")
	resp, err := c.doer.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseMatchRefs(body)
}

func parseMatchRefs(body []byte) ([]matchstore.MatchRef, error) {
	var env struct {
		Rows []map[string]json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	if len(env.Rows) == 0 {
		return nil, nil
	}
	var out []matchstore.MatchRef
	for _, row := range env.Rows {
		// Each row must have match_id and start_time for partition-pruned
		// UnknownIDs queries.  The SQL query in assets/queries/matches/
		// returns both columns.
		var id int64
		rm, ok := row["match_id"]
		if !ok {
			// Fallback: try match_ids array (legacy format)
			if rm, ok := row["match_ids"]; ok {
				var arr []json.RawMessage
				if err := json.Unmarshal(rm, &arr); err == nil {
					for _, v := range arr {
						if ids := extractID(v); len(ids) > 0 {
							out = append(out, matchstore.MatchRef{MatchID: ids[0]})
						}
					}
				}
			}
			continue
		}
		ids := extractID(rm)
		if len(ids) == 0 {
			continue
		}
		id = ids[0]

		// Extract start_time for partition pruning.
		var startTime int64
		if st, ok := row["start_time"]; ok {
			startTime = extractInt64(st)
		}
		out = append(out, matchstore.MatchRef{MatchID: id, StartTime: startTime})
	}
	return out, nil
}

// extractID parses a match ID from a raw JSON value that may be a string
// (e.g. the OpenData explorer returns bigint columns as strings) or a number.
func extractID(r json.RawMessage) []int64 {
	var s string
	if err := json.Unmarshal(r, &s); err == nil {
		if n, perr := strconv.ParseInt(s, 10, 64); perr == nil && n > 0 {
			return []int64{n}
		}
		return nil
	}
	var n int64
	if err := json.Unmarshal(r, &n); err == nil && n > 0 {
		return []int64{n}
	}
	return nil
}

// extractInt64 parses a numeric value that may be a string or number.
func extractInt64(r json.RawMessage) int64 {
	var s string
	if err := json.Unmarshal(r, &s); err == nil {
		if n, perr := strconv.ParseInt(s, 10, 64); perr == nil {
			return n
		}
		return 0
	}
	var n int64
	if err := json.Unmarshal(r, &n); err == nil {
		return n
	}
	return 0
}