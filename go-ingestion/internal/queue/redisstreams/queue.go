package redisstreams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/user-for-download/go-dota2/internal/queue"
)

type AsyncRetryConfig struct {
	ZSetKey    string
	PollEvery  time.Duration
	BatchSize  int64
	MaxRetries int
}

type Config struct {
	Stream      string
	DLQStream   string
	Group       string
	Consumer    string
	MaxLen      int64
	Policy      queue.RetryPolicy
	DeleteOnAck bool
	Logger      *slog.Logger
}

const (
	fieldPayload = "p"
	fieldRetry   = "r"
	fieldReason  = "reason"
)

type Queue struct {
	rdb           *goredis.Client
	cfg           Config
	log           *slog.Logger
	recoverCursor string
	propagator    propagation.TextMapPropagator
	asyncCfg      AsyncRetryConfig
	asyncStopCh   chan struct{}
	asyncStarted  bool
	asyncMu       sync.Mutex
}

func New(rdb *goredis.Client, cfg Config) (*Queue, error) {
	if rdb == nil {
		return nil, fmt.Errorf("redisstreams: nil redis client")
	}
	if cfg.Stream == "" {
		return nil, fmt.Errorf("redisstreams: Stream is required")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	q := &Queue{
		rdb:           rdb,
		cfg:           cfg,
		log:           log.With("queue", cfg.Stream),
		recoverCursor: "0-0",
		propagator:    otel.GetTextMapPropagator(),
	}

	if cfg.Consumer != "" && cfg.Group != "" {
		if err := q.ensureGroup(context.Background()); err != nil {
			return nil, err
		}
		q.recoverCursor = "0-0"
	}
	return q, nil
}

var _ queue.Queue = (*Queue)(nil)
var _ queue.QueueObservable = (*Queue)(nil)

func (q *Queue) ensureGroup(ctx context.Context) error {
	err := q.rdb.XGroupCreateMkStream(ctx, q.cfg.Stream, q.cfg.Group, "$").Err()
	if err == nil || isBusyGroup(err) {
		return nil
	}
	return fmt.Errorf("xgroup create: %w", err)
}

func (q *Queue) Push(ctx context.Context, payload []byte) error {
	cp := append([]byte(nil), payload...)
	values := map[string]any{
		fieldPayload: cp,
		fieldRetry:   "0",
	}

	carrier := propagation.MapCarrier{}
	q.propagator.Inject(ctx, carrier)
	for k, v := range carrier {
		values["_otel_"+k] = v
	}

	args := &goredis.XAddArgs{
		Stream: q.cfg.Stream,
		Values: values,
	}
	if q.cfg.MaxLen > 0 {
		args.MaxLen = q.cfg.MaxLen
		args.Approx = true
	}
	if err := q.rdb.XAdd(ctx, args).Err(); err != nil {
		return fmt.Errorf("xadd: %w", err)
	}
	return nil
}

func (q *Queue) Pop(ctx context.Context, batch int, block time.Duration) ([]queue.Task, error) {
	if q.cfg.Consumer == "" || q.cfg.Group == "" {
		return nil, fmt.Errorf("redisstreams: Consumer and Group required for Pop")
	}
	if batch <= 0 {
		batch = 1
	}
	res, err := q.rdb.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    q.cfg.Group,
		Consumer: q.cfg.Consumer,
		Streams:  []string{q.cfg.Stream, ">"},
		Count:    int64(batch),
		Block:    block,
	}).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, queue.ErrEmpty
	}
	if err != nil {
		return nil, fmt.Errorf("xreadgroup: %w", err)
	}
	if len(res) == 0 || len(res[0].Messages) == 0 {
		return nil, queue.ErrEmpty
	}

	out := make([]queue.Task, 0, len(res[0].Messages))
	for _, msg := range res[0].Messages {
		t, err := decodeMessage(msg, q.propagator)
		if err != nil {
			q.log.Warn("decode failed; routing to DLQ", "id", msg.ID, "err", err)
			_ = q.routeDLQ(ctx, queue.Task{ID: msg.ID}, "decode_error: "+err.Error())
			continue
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, queue.ErrEmpty
	}
	return out, nil
}

func (q *Queue) Ack(ctx context.Context, taskID string) error {
	if err := q.rdb.XAck(ctx, q.cfg.Stream, q.cfg.Group, taskID).Err(); err != nil {
		return fmt.Errorf("xack: %w", err)
	}
	if q.cfg.DeleteOnAck {
		_ = q.rdb.XDel(ctx, q.cfg.Stream, taskID).Err()
	}
	return nil
}

func (q *Queue) Retry(ctx context.Context, t queue.Task, reason string) error {
	t.RetryCount++

	if q.cfg.Policy.ShouldDLQ(t.RetryCount) {
		return q.routeDLQ(ctx, t, reason)
	}

	if q.asyncCfg.ZSetKey != "" {
		return q.scheduleAsyncRetry(ctx, t)
	}

	d := q.cfg.Policy.Backoff(t.RetryCount)
	if d > 0 {
		timer := time.NewTimer(d)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
		timer.Stop()
	}
	return q.requeue(ctx, t)
}

func (q *Queue) scheduleAsyncRetry(ctx context.Context, t queue.Task) error {
	if q.asyncCfg.MaxRetries > 0 && t.RetryCount > q.asyncCfg.MaxRetries {
		return q.routeDLQ(ctx, t, "max retries exceeded")
	}

	var raw map[string]any
	if err := json.Unmarshal(t.Payload, &raw); err != nil {
		raw = map[string]any{"p": string(t.Payload)}
	}
	raw["retry_count"] = t.RetryCount

	payload, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal retry task: %w", err)
	}

	d := q.cfg.Policy.Backoff(t.RetryCount)
	if d <= 0 {
		d = time.Second
	}
	score := float64(time.Now().Add(d).Unix())

	pipe := q.rdb.Pipeline()
	pipe.ZAdd(ctx, q.asyncCfg.ZSetKey, goredis.Z{Score: score, Member: string(payload)})
	if t.ID != "" {
		pipe.XAck(ctx, q.cfg.Stream, q.cfg.Group, t.ID)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("schedule async retry: %w", err)
	}
	return nil
}

func (q *Queue) routeDLQ(ctx context.Context, t queue.Task, reason string) error {
	if q.cfg.DLQStream == "" {
		q.log.Warn("DLQ not configured; dropping task", "id", t.ID, "reason", reason, "retries", t.RetryCount)
	} else {
		err := q.rdb.XAdd(ctx, &goredis.XAddArgs{
			Stream: q.cfg.DLQStream,
			Values: map[string]any{
				fieldPayload: t.Payload,
				fieldRetry:   strconv.Itoa(t.RetryCount),
				fieldReason:  reason,
			},
		}).Err()
		if err != nil {
			return fmt.Errorf("xadd dlq: %w", err)
		}
	}
	if t.ID == "" {
		return nil
	}
	return q.Ack(ctx, t.ID)
}

func (q *Queue) requeue(ctx context.Context, t queue.Task) error {
	args := &goredis.XAddArgs{
		Stream: q.cfg.Stream,
		Values: map[string]any{
			fieldPayload: t.Payload,
			fieldRetry:   strconv.Itoa(t.RetryCount),
		},
	}
	if q.cfg.MaxLen > 0 {
		args.MaxLen = q.cfg.MaxLen
		args.Approx = true
	}
	if err := q.rdb.XAdd(ctx, args).Err(); err != nil {
		return fmt.Errorf("xadd requeue: %w", err)
	}
	return q.Ack(ctx, t.ID)
}

func (q *Queue) RecoverStale(ctx context.Context, idleFor time.Duration, max int) ([]queue.Task, error) {
	if q.cfg.Consumer == "" || q.cfg.Group == "" {
		return nil, fmt.Errorf("redisstreams: Consumer and Group required for RecoverStale")
	}
	if max <= 0 {
		max = 100
	}
	args := &goredis.XAutoClaimArgs{
		Stream:   q.cfg.Stream,
		Group:    q.cfg.Group,
		Consumer: q.cfg.Consumer,
		MinIdle:  idleFor,
		Start:    q.recoverCursor,
		Count:    int64(max),
	}
	res, nextCursor, err := q.rdb.XAutoClaim(ctx, args).Result()
	if err != nil {
		return nil, fmt.Errorf("xautoclaim: %w", err)
	}
	q.recoverCursor = nextCursor
	if q.recoverCursor == "" || q.recoverCursor == "0-0" {
		q.recoverCursor = "0-0"
	}
	out := make([]queue.Task, 0, len(res))
	for _, msg := range res {
		t, err := decodeMessage(msg, q.propagator)
		if err != nil {
			q.log.Warn("decode failed during recover; routing to DLQ", "id", msg.ID, "err", err)
			_ = q.routeDLQ(ctx, queue.Task{ID: msg.ID}, "decode_error: "+err.Error())
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func decodeMessage(msg goredis.XMessage, propagator propagation.TextMapPropagator) (queue.Task, error) {
	var t queue.Task
	t.ID = msg.ID

	rawPayload, ok := msg.Values[fieldPayload]
	if !ok {
		return t, fmt.Errorf("missing payload field %q", fieldPayload)
	}
	switch v := rawPayload.(type) {
	case string:
		t.Payload = []byte(v)
	case []byte:
		t.Payload = v
	default:
		return t, fmt.Errorf("unexpected payload type %T", v)
	}

	if rawRetry, ok := msg.Values[fieldRetry]; ok {
		if s, ok := rawRetry.(string); ok {
			if n, err := strconv.Atoi(s); err == nil {
				t.RetryCount = n
			}
		}
	}

	carrier := propagation.MapCarrier{}
	for k, v := range msg.Values {
		if strings.HasPrefix(k, "_otel_") {
			if strVal, ok := v.(string); ok {
				carrier[strings.TrimPrefix(k, "_otel_")] = strVal
			}
		}
	}
	t.Ctx = propagator.Extract(context.Background(), carrier)

	return t, nil
}

func isBusyGroup(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "BUSYGROUP")
}

func (q *Queue) PendingLen() int64 {
	if q.cfg.Stream == "" {
		return 0
	}
	n, err := q.rdb.XLen(context.Background(), q.cfg.Stream).Result()
	if err != nil {
		q.log.Debug("xlen failed", "err", err)
		return 0
	}
	return n
}

func (q *Queue) InFlightLen() int64 {
	if q.cfg.Stream == "" || q.cfg.Group == "" {
		return 0
	}
	n, err := q.rdb.XPending(context.Background(), q.cfg.Stream, q.cfg.Group).Result()
	if err != nil {
		q.log.Debug("xpending failed", "err", err)
		return 0
	}
	return n.Count
}

func (q *Queue) Stats(ctx context.Context) (queue.QueueStats, error) {
	return queue.QueueStats{
		Pending:  q.PendingLen(),
		InFlight: q.InFlightLen(),
	}, nil
}

func (q *Queue) EnableAsyncRetry(cfg AsyncRetryConfig) {
	q.asyncMu.Lock()
	defer q.asyncMu.Unlock()
	if q.asyncStarted {
		return
	}
	q.asyncCfg = cfg
	if q.asyncCfg.PollEvery <= 0 {
		q.asyncCfg.PollEvery = 1 * time.Second
	}
	if q.asyncCfg.BatchSize <= 0 {
		q.asyncCfg.BatchSize = 100
	}
	q.asyncStopCh = make(chan struct{})
	q.asyncStarted = true
	go q.asyncRetryLoop()
}

func (q *Queue) StopAsyncRetry() {
	q.asyncMu.Lock()
	defer q.asyncMu.Unlock()
	if !q.asyncStarted {
		return
	}
	close(q.asyncStopCh)
	q.asyncStarted = false
}

func (q *Queue) asyncRetryLoop() {
	ticker := time.NewTicker(q.asyncCfg.PollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-q.asyncStopCh:
			return
		case <-ticker.C:
			q.matureAsyncRetries(context.Background())
		}
	}
}

func (q *Queue) matureAsyncRetries(ctx context.Context) {
	if q.asyncCfg.ZSetKey == "" {
		return
	}
	now := float64(time.Now().Unix())
	res, err := q.rdb.ZRangeByScoreWithScores(ctx, q.asyncCfg.ZSetKey, &goredis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%f", now),
		Count: q.asyncCfg.BatchSize,
	}).Result()
	if err != nil {
		q.log.Debug("async retry: zrangebyscore failed", "err", err)
		return
	}
	if len(res) == 0 {
		return
	}

	pipe := q.rdb.Pipeline()
	for _, z := range res {
		member, ok := z.Member.(string)
		if !ok {
			continue
		}
		// Unwrap the payload: scheduleAsyncRetry stores {"p":"...","retry_count":N}.
		// Extract the original "p" field so handlers receive the expected payload.
		payload := extractOriginalPayload(member)
		retryCount := extractRetryCount(member)
		pipe.XAdd(ctx, &goredis.XAddArgs{
			Stream: q.cfg.Stream,
			Values: map[string]any{
				fieldPayload: payload,
				fieldRetry:   retryCount,
			},
		})
		pipe.ZRem(ctx, q.asyncCfg.ZSetKey, z.Member)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		q.log.Warn("async retry: pipe exec failed", "err", err)
	}
}

func extractRetryCount(payload string) string {
	var t struct {
		RetryCount int `json:"retry_count"`
	}
	if err := json.Unmarshal([]byte(payload), &t); err != nil {
		return "0"
	}
	return strconv.Itoa(t.RetryCount)
}

// extractOriginalPayload unwraps the payload stored by scheduleAsyncRetry.
// scheduleAsyncRetry stores {"p":"<original>","retry_count":N}.
// This function extracts the original "p" field so handlers receive
// the expected payload format.
func extractOriginalPayload(wrapped string) string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(wrapped), &raw); err != nil {
		return wrapped
	}
	if p, ok := raw["p"]; ok {
		switch v := p.(type) {
		case string:
			return v
		case []byte:
			return string(v)
		}
	}
	return wrapped
}