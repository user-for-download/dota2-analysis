package proxy

import "errors"

var (
	ErrRateLimited = errors.New("proxy: rate limited")
	ErrNoProxy     = errors.New("proxy: none available")
)
