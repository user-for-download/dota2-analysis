package proxy

import "time"

type RateLimit struct {
	RatePerSec int
	Burst      int
	Window     time.Duration
}

func (r RateLimit) Disabled() bool {
	return r.RatePerSec <= 0 || r.Burst <= 0
}

type Ranking struct {
	FailurePenalty float64
	SuccessBoost   float64
	InitialWeight  float64
}

func (r Ranking) WithDefaults() Ranking {
	if r.InitialWeight == 0 {
		r.InitialWeight = 100
	}
	if r.SuccessBoost == 0 {
		r.SuccessBoost = 1
	}
	if r.FailurePenalty == 0 {
		r.FailurePenalty = 5
	}
	return r
}
