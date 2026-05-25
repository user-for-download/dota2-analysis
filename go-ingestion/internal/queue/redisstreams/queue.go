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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/user-for-download/dota2-analysis/go-ingestion/internal/queue"
)

// LatencySink records per-message processing latency.
// Pass a minimal adapter from the consumer; avoids importing the full metrics package.
type LatencySink interface {
	RecordLatency(ctx context.Context, stage string, durationMs float64)
}

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

	// Subscribe settings (optional, only used by Subscribe).
	SubscribeBatch   int           // Pop batch size (default 10)
	SubscribeBlock   time.Duration // Pop block duration (default 2s)
	SubscribeRecover bool          // run periodic stale-recovery goroutine
	SubscribeStage   string        // stage label for latency metric
	LatencySink      LatencySink   // records per-message latency
}

const (
	fieldPayload = "p"
	fieldRetry   = "r"
	fieldReason  = "reason"
)

// luaRequeueAtomic atomically adds a new stream entry, acks the old one,
// and deletes the old entry. Without this, a crash between XAdd and XAck
// duplicates the message.
//
//	KEYS[1] = stream, KEYS[2] = group
//	ARGV[1] = maxLen, ARGV[2] = old task ID to ack, ARGV[3..] = field-value pairs
var luaRequeueAtomic = goredis.NewScript(`
redis.call('XADD', KEYS[1], 'MAXLEN', '~', ARGV[1], '*', unpack(ARGV, 3))
redis.call('XACK', KEYS[1], KEYS[2], ARGV[2])
redis.call('XDEL', KEYS[1], ARGV[2])
return 1
`)

// luaRouteDLQAtomic atomically adds to DLQ and acks/deletes the original.
//
//	KEYS[1] = DLQ stream, KEYS[2] = original stream, KEYS[3] = group
//	ARGV[1] = maxLen, ARGV[2] = original task ID, ARGV[3..] = field-value pairs
var luaRouteDLQAtomic = goredis.NewScript(`
redis.call('XADD', KEYS[1], 'MAXLEN', '~', ARGV[1], '*', unpack(ARGV, 3))
redis.call('XACK', KEYS[2], KEYS[3], ARGV[2])
redis.call('XDEL', KEYS[2], ARGV[2])
return 1
`)

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
var _ queue.Publisher = (*Queue)(nil)
var _ queue.Subscriber = (*Queue)(nil)

func (q *Queue) ensureGroup(ctx context.Context) error {
	err := q.rdb.XGroupCreateMkStream(ctx, q.cfg.Stream, q.cfg.Group, "$").Err()
	if err == nil || isBusyGroup(err) {
		return nil
	}
	return fmt.Errorf("xgroup create: %w", err)
}

// Publish implements queue.Publisher.  Delegates to Push.
func (q *Queue) Publish(ctx context.Context, payload []byte) error {
	return q.Push(ctx, payload)
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
			if dlqErr := q.routeDLQ(ctx, queue.Task{ID: msg.ID}, "decode_error: "+err.Error()); dlqErr != nil {
				q.log.Error("DLQ routing failed; message remains pending (will retry)",
					"id", msg.ID, "dlq_err", dlqErr,
				)
			}
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

	// No async retry configured — requeue immediately without blocking the
	// subscriber loop. Backoff is naturally approximated by the retry count
	// since each retry cycles through the full Pop → handle → Retry loop.
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
		if t.ID != "" {
			_ = q.Ack(ctx, t.ID)
		}
		return nil
	}

	if t.ID == "" {
		// No original message to ack; just add to DLQ.
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
		return nil
	}

	// Atomic DLQ + ack to prevent duplication on crash.
	keys := []string{q.cfg.DLQStream, q.cfg.Stream, q.cfg.Group}
	args := []any{q.cfg.MaxLen, t.ID,
		fieldPayload, t.Payload,
		fieldRetry, strconv.Itoa(t.RetryCount),
		fieldReason, reason,
	}
	_, err := luaRouteDLQAtomic.Run(ctx, q.rdb, keys, args...).Int64()
	if err != nil {
		return fmt.Errorf("atomic dlq: %w", err)
	}
	return nil
}

func (q *Queue) requeue(ctx context.Context, t queue.Task) error {
	keys := []string{q.cfg.Stream, q.cfg.Group}
	args := []any{q.cfg.MaxLen, t.ID,
		fieldPayload, t.Payload,
		fieldRetry, strconv.Itoa(t.RetryCount),
	}
	_, err := luaRequeueAtomic.Run(ctx, q.rdb, keys, args...).Int64()
	if err != nil {
		return fmt.Errorf("atomic requeue: %w", err)
	}
	return nil
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
			if dlqErr := q.routeDLQ(ctx, queue.Task{ID: msg.ID}, "decode_error: "+err.Error()); dlqErr != nil {
				q.log.Error("DLQ routing failed during recover; message remains pending",
					"id", msg.ID, "dlq_err", dlqErr,
				)
			}
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

// StreamLen returns the total number of entries in the stream (including
// acknowledged messages that have not yet been trimmed). For the count of
// unacknowledged messages, see InFlightLen.
func (q *Queue) StreamLen() int64 {
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
		StreamLen: q.StreamLen(),
		InFlight:  q.InFlightLen(),
	}, nil
}

// Subscribe implements queue.Subscriber.  It runs the Pop → handle → Ack/Retry
// loop, handles backpressure, stale recovery, OTel tracing, and panic recovery.
// Blocks until ctx is cancelled or a terminal error occurs.
func (q *Queue) Subscribe(ctx context.Context, handler queue.Handler) error {
	batch := q.cfg.SubscribeBatch
	if batch <= 0 {
		batch = 10
	}
	block := q.cfg.SubscribeBlock
	if block <= 0 {
		block = 2 * time.Second
	}
	baseBatch := batch
	baseBlock := block

	log := q.log.With("component", "redisstreams.subscriber")
	tracer := otel.Tracer("redisstreams")

	// Start stale recovery if configured
	if q.cfg.SubscribeRecover {
		go q.staleRecoveryLoop(ctx)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Backpressure: reduce batch / increase block when queue is deep
		batch, block = q.adjustForBackpressure(batch, block, baseBatch, baseBlock, log)

		tasks, err := q.Pop(ctx, batch, block)
		if errors.Is(err, queue.ErrEmpty) {
			continue
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			log.Warn("queue pop", "err", err)
			continue
		}

		for _, t := range tasks {
			var retryWrapper struct {
				Retry *int `json:"_retry"`
			}
			if err := json.Unmarshal(t.Payload, &retryWrapper); err == nil && retryWrapper.Retry != nil {
				if *retryWrapper.Retry > t.RetryCount {
					t.RetryCount = *retryWrapper.Retry
				}
			}

			spanCtx := t.Ctx
			if spanCtx == nil {
				spanCtx = ctx
			}

			spanCtx, span := tracer.Start(spanCtx, "subscriber.process",
				trace.WithAttributes(
					attribute.String("task.id", t.ID),
					attribute.Int("task.retry_count", t.RetryCount),
				),
			)
			start := time.Now()

			msg := queue.Message{ID: t.ID, Payload: t.Payload}
			var handlerErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						handlerErr = fmt.Errorf("handler panic: %v", r)
					}
				}()
				handlerErr = handler(spanCtx, msg)
			}()

			if q.cfg.LatencySink != nil {
				q.cfg.LatencySink.RecordLatency(spanCtx, q.cfg.SubscribeStage, float64(time.Since(start).Milliseconds()))
			}

			if handlerErr != nil {
				span.RecordError(handlerErr)
				span.SetStatus(codes.Error, handlerErr.Error())
			} else {
				span.SetStatus(codes.Ok, "success")
			}
			span.End()

			switch {
			case handlerErr == nil:
				_ = q.Ack(ctx, t.ID)
			case errors.Is(handlerErr, queue.ErrDrop):
				_ = q.Ack(ctx, t.ID)
			default:
				_ = q.Retry(ctx, t, handlerErr.Error())
			}
		}
	}
}

func (q *Queue) staleRecoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	log := q.log.With("component", "redisstreams.stale_recovery")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tasks, err := q.RecoverStale(ctx, 10*time.Minute, 100)
			if err != nil {
				log.Warn("stale recovery", "err", err)
				continue
			}
			if len(tasks) == 0 {
				continue
			}
			requeued := 0
			dropped := 0
			for _, t := range tasks {
				payload := withRetryCount(t.Payload, t.RetryCount)
				if err := q.Push(ctx, payload); err != nil {
					log.Warn("stale re-enqueue failed", "id", t.ID, "err", err)
					dropped++
					continue
				}
				_ = q.Ack(ctx, t.ID)
				requeued++
			}
			log.Info("recovered stale", "count", len(tasks), "requeued", requeued, "dropped", dropped)
		}
	}
}

const (
	maxBlockDuration = 5 * time.Second
	recoverThreshold = 1000
)

func (q *Queue) adjustForBackpressure(batch int, block time.Duration, baseBatch int, baseBlock time.Duration, log *slog.Logger) (int, time.Duration) {
	pending := q.StreamLen()
	inFlight := q.InFlightLen()

	if pending > 10000 {
		batch = max(batch/2, 1)
		block = min(block*2, maxBlockDuration)
		log.Debug("backpressure: reducing rate",
			"pending", pending, "in_flight", inFlight,
			"new_batch", batch, "new_block", block)
	} else if pending < recoverThreshold && (batch < baseBatch || block > baseBlock) {
		batch = min(batch+1, baseBatch)
		if block > baseBlock {
			block = max(block/2, baseBlock)
		}
		log.Debug("backpressure: recovering",
			"pending", pending, "in_flight", inFlight,
			"new_batch", batch, "new_block", block)
	} else if batch != baseBatch || block != baseBlock {
		log.Debug("backpressure: stable",
			"pending", pending, "in_flight", inFlight,
			"batch", batch, "block", block,
			"base_batch", baseBatch, "base_block", baseBlock)
	}
	return batch, block
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

// withRetryCount injects the retry count into a JSON payload as _retry.
// Used by stale recovery to preserve retry state across re-enqueue.
func withRetryCount(payload []byte, retryCount int) []byte {
	if retryCount <= 0 {
		return payload
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return payload
	}
	raw["_retry"] = retryCount
	out, err := json.Marshal(raw)
	if err != nil {
		return payload
	}
	return out
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