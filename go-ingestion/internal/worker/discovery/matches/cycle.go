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

	"github.com/user-for-download/go-dota2/internal/dedup"
	"github.com/user-for-download/go-dota2/internal/metrics"
	"github.com/user-for-download/go-dota2/internal/queue"
	"github.com/user-for-download/go-dota2/internal/worker/discovery"
	"github.com/user-for-download/go-dota2/internal/worker/fetcher"
)

type Config struct {
	ExplorerURL string
	Queries    map[string]string
	DefaultKey string
	Interval  time.Duration
	RunAtStart bool
	Logger   *slog.Logger
	Dedup    dedup.Seen
	FileKey  string
	Doer    discovery.HTTPDoer
}

type Cycle struct {
	out   queue.Queue
	doer  discovery.HTTPDoer
	m     metrics.Sink
	dedup dedup.Seen
	cfg   Config
	log   *slog.Logger
}

func New(out queue.Queue, doer discovery.HTTPDoer, m metrics.Sink, cfg Config) (*Cycle, error) {
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
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Cycle{
		out:   out,
		doer:  doer,
		m:     m,
		dedup: cfg.Dedup,
		cfg:   cfg,
		log:   log.With("component", "discovery.matches"),
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

	ids, err := c.fetchMatchIDs(ctx, sql)
	if err != nil {
		c.m.FetchFailure(ctx, metrics.KindHTTP)
		return fmt.Errorf("fetch match ids (%s): %w", key, err)
	}
	c.log.Info("query returned", "key", key, "count", len(ids))

	pushed := 0
	skipped := 0
	for _, id := range ids {
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
		if err := c.out.Push(ctx, payload); err != nil {
			c.log.Warn("queue push", "match_id", id, "err", err)
			continue
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