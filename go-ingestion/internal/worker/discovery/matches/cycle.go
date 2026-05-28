package matches

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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

	var ids []int64
	var err error
	const maxBackoff = 30 * time.Second
	for attempt := 1; ctx.Err() == nil; attempt++ {
		c.log.Debug("discoverer: fetching match ids", "key", key, "attempt", attempt)
		ids, err = c.fetchMatchIDs(ctx, sql)
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	if err != nil {
		return fmt.Errorf("fetch match ids (%s): %w", key, err)
	}
	c.log.Info("query returned", "key", key, "count", len(ids))

	// Pre-filter with PostgreSQL to respect existing parsed matches even if Redis resets.
	c.log.Debug("discoverer: filtering against db", "candidates", len(ids))
	if c.reader != nil && len(ids) > 0 {
		unknownIDs, err := c.reader.UnknownIDs(ctx, ids)
		if err != nil {
			c.log.Warn("failed to check unknown ids against db", "err", err)
		} else {
			c.log.Info("filtered discovered matches against db", "original", len(ids), "unknown", len(unknownIDs))
			ids = unknownIDs
		}
	}

	pushed := 0
	skipped := 0
	for _, id := range ids {
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
	c.log.Info("pushed tasks", "key", key, "pushed", pushed, "skipped", skipped, "discovered", len(ids))
	return nil
}

func (c *Cycle) fetchMatchIDs(ctx context.Context, sql string) ([]int64, error) {
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
	return parseMatchIDs(body)
}

func parseMatchIDs(body []byte) ([]int64, error) {
	var env struct {
		Rows []struct {
			MatchIDs []json.RawMessage `json:"match_ids"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	if len(env.Rows) == 0 {
		return nil, nil
	}
	var out []int64
	for _, row := range env.Rows {
		for _, r := range row.MatchIDs {
			var s string
			if err := json.Unmarshal(r, &s); err == nil {
				n, perr := strconv.ParseInt(s, 10, 64)
				if perr != nil || n <= 0 {
					continue
				}
				out = append(out, n)
				continue
			}
			var n int64
			if err := json.Unmarshal(r, &n); err == nil && n > 0 {
				out = append(out, n)
			}
		}
	}
	return out, nil
}