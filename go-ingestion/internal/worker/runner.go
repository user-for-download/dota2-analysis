package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/user-for-download/go-dota2/internal/metrics"
	"github.com/user-for-download/go-dota2/internal/queue"
)

type Result string

const (
	ResultSuccess Result = "success"
	ResultRetry   Result = "retry"
	ResultDrop    Result = "drop"
)

type Handler interface {
	Process(ctx context.Context, t queue.Task) (Result, error)
}

type HTTPDoer interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

type MetricsSink interface {
	RecordLatency(ctx context.Context, stage metrics.Stage, durationMs float64)
}

type StageMetrics struct {
	Stage metrics.Stage
}

type Config struct {
	Batch          int
	Block          time.Duration
	Logger         *slog.Logger
	RecoverStale   bool
	QueueProvider  QueueDepthProvider
	Metrics        MetricsSink
}

type QueueDepthProvider interface {
	PendingLen() int64
	InFlightLen() int64
}

type CircuitBreaker struct {
	failures              atomic.Int64
	successes             atomic.Int64
	threshold             int64
	halfOpenAfter         time.Duration
	halfOpenSuccessTarget int64
	state                 int32
	timerCh               atomic.Value
}

const (
	circuitClosed    int32 = 0
	circuitOpen      int32 = 1
	circuitHalfOpen  int32 = 2
)

func NewCircuitBreaker(threshold int64, halfOpenAfter time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:             threshold,
		halfOpenAfter:         halfOpenAfter,
		halfOpenSuccessTarget: 3,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	switch atomic.LoadInt32(&cb.state) {
	case circuitOpen:
		return false
	case circuitHalfOpen:
		return true
	default:
		return true
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	state := atomic.LoadInt32(&cb.state)
	switch state {
	case circuitOpen:
		return
	case circuitHalfOpen:
		if prev, ok := cb.timerCh.Load().(chan struct{}); ok {
			select {
			case <-prev:
			default:
				close(prev)
			}
		}
		if !atomic.CompareAndSwapInt32(&cb.state, circuitHalfOpen, circuitOpen) {
			return
		}
		cb.successes.Store(0)
		cb.failures.Store(0)
		next := make(chan struct{})
		cb.timerCh.Store(next)
		go func(stop chan struct{}) {
			select {
			case <-stop:
				return
			case <-time.After(cb.halfOpenAfter):
				atomic.StoreInt32(&cb.state, circuitHalfOpen)
			}
		}(next)
		return
	}
	n := cb.failures.Add(1)
	if n >= cb.threshold && atomic.CompareAndSwapInt32(&cb.state, circuitClosed, circuitOpen) {
		cb.successes.Store(0)
		cb.failures.Store(0)
		if prev, ok := cb.timerCh.Load().(chan struct{}); ok {
			select {
			case <-prev:
			default:
				close(prev)
			}
		}
		next := make(chan struct{})
		cb.timerCh.Store(next)
		go func(stop chan struct{}) {
			select {
			case <-stop:
				return
			case <-time.After(cb.halfOpenAfter):
				atomic.StoreInt32(&cb.state, circuitHalfOpen)
			}
		}(next)
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	if atomic.LoadInt32(&cb.state) != circuitHalfOpen {
		return
	}
	s := cb.successes.Add(1)
	if s >= cb.halfOpenSuccessTarget {
		if !atomic.CompareAndSwapInt32(&cb.state, circuitHalfOpen, circuitClosed) {
			return
		}
		cb.successes.Store(0)
	}
}

func Run(ctx context.Context, q queue.Queue, cfg Config, stage metrics.Stage, h Handler) error {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	log = log.With("component", "worker.runner")

	tracer := otel.Tracer("worker")

	if cfg.RecoverStale {
		if rs, ok := q.(interface {
			RecoverStale(ctx context.Context, idleFor time.Duration, max int) ([]queue.Task, error)
		}); ok {
			go staleRecoveryLoop(ctx, q, rs, log)
		}
	}

	baseBatch := cfg.Batch
	baseBlock := cfg.Block
	if baseBatch <= 0 {
		baseBatch = 10
	}
	if baseBlock <= 0 {
		baseBlock = 2 * time.Second
	}
	batch := baseBatch
	block := baseBlock

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if cfg.QueueProvider != nil {
			batch, block = adjustForBackpressure(cfg.QueueProvider, batch, block, baseBatch, baseBlock, log)
		}
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
			// Transparently extract _retry from payload if injected by stale recovery.
			// This keeps the Worker Handlers blissfully unaware of Queue mechanics.
			var retryWrapper struct {
				Retry *int `json:"_retry"`
			}
			if err := json.Unmarshal(t.Payload, &retryWrapper); err == nil && retryWrapper.Retry != nil {
				if *retryWrapper.Retry > t.RetryCount {
					t.RetryCount = *retryWrapper.Retry
				}
			}

			taskCtx := t.Ctx
			if taskCtx == nil {
				taskCtx = ctx
			}

			spanCtx, span := tracer.Start(taskCtx, "worker.process",
				trace.WithAttributes(
					attribute.String("task.id", t.ID),
					attribute.Int("task.retry_count", t.RetryCount),
				),
			)
			start := time.Now()
			var procErr error
			var result Result
			func() {
				defer func() {
					if r := recover(); r != nil {
						procErr = fmt.Errorf("process panic: %v", r)
						result = ResultRetry
					}
				}()
				result, procErr = h.Process(spanCtx, t)
			}()
			if cfg.Metrics != nil {
				cfg.Metrics.RecordLatency(spanCtx, stage, float64(time.Since(start).Milliseconds()))
			}
			if procErr != nil {
				span.RecordError(procErr)
				span.SetStatus(codes.Error, procErr.Error())
			} else {
				span.SetStatus(codes.Ok, "success")
			}
			span.End()

			switch result {
			case ResultSuccess:
				_ = q.Ack(ctx, t.ID)
			case ResultDrop:
				_ = q.Ack(ctx, t.ID)
			case ResultRetry:
				reason := "retry"
				if procErr != nil {
					reason = procErr.Error()
				}
				_ = q.Retry(ctx, t, reason)
			default:
				if procErr != nil {
					_ = q.Retry(ctx, t, procErr.Error())
				} else {
					_ = q.Ack(ctx, t.ID)
				}
			}
		}
}
}

const (
	maxBlockDuration = 5 * time.Second
	recoverThreshold = 1000
)

func adjustForBackpressure(q QueueDepthProvider, batch int, block time.Duration, baseBatch int, baseBlock time.Duration, log *slog.Logger) (int, time.Duration) {
	pending := q.PendingLen()
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

func staleRecoveryLoop(ctx context.Context, q queue.Queue, rs interface {
	RecoverStale(ctx context.Context, idleFor time.Duration, max int) ([]queue.Task, error)
}, log *slog.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tasks, err := rs.RecoverStale(ctx, 10*time.Minute, 100)
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
				// Ack the XAUTOCLAIMed message to prevent it from lingering
				// in the pending list forever.
				_ = q.Ack(ctx, t.ID)
				requeued++
			}
			log.Info("recovered stale", "count", len(tasks), "requeued", requeued, "dropped", dropped)
		}
	}
}

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

