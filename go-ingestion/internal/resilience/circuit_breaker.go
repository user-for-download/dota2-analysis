package resilience

import (
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreaker is a state machine that fails fast when a threshold of errors is reached.
type CircuitBreaker struct {
	failures              atomic.Int64
	successes             atomic.Int64
	threshold             int64
	halfOpenAfter         time.Duration
	halfOpenSuccessTarget int64
	state                 int32

	mu      sync.Mutex
	stopCh  chan struct{}
}

const (
	circuitClosed   int32 = 0
	circuitOpen     int32 = 1
	circuitHalfOpen int32 = 2
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
		cb.stopTimer()
		if !atomic.CompareAndSwapInt32(&cb.state, circuitHalfOpen, circuitOpen) {
			return
		}
		cb.successes.Store(0)
		cb.failures.Store(0)
		cb.startTimer()
		return
	}
	n := cb.failures.Add(1)
	if n >= cb.threshold && atomic.CompareAndSwapInt32(&cb.state, circuitClosed, circuitOpen) {
		cb.successes.Store(0)
		cb.failures.Store(0)
		cb.stopTimer()
		cb.startTimer()
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	state := atomic.LoadInt32(&cb.state)
	if state == circuitHalfOpen {
		s := cb.successes.Add(1)
		if s >= cb.halfOpenSuccessTarget {
			if !atomic.CompareAndSwapInt32(&cb.state, circuitHalfOpen, circuitClosed) {
				return
			}
			cb.successes.Store(0)
			cb.failures.Store(0)
		}
		return
	}
	if state == circuitClosed {
		for {
			cur := cb.failures.Load()
			if cur <= 0 {
				break
			}
			if cb.failures.CompareAndSwap(cur, cur-1) {
				break
			}
		}
	}
}

func (cb *CircuitBreaker) stopTimer() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.stopCh != nil {
		close(cb.stopCh)
		cb.stopCh = nil
	}
}

func (cb *CircuitBreaker) startTimer() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	next := make(chan struct{})
	cb.stopCh = next
	go func(stop chan struct{}) {
		select {
		case <-stop:
			return
		case <-time.After(cb.halfOpenAfter):
			atomic.StoreInt32(&cb.state, circuitHalfOpen)
		}
	}(next)
}
