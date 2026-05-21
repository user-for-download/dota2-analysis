package fetcher

import (
	"fmt"
	"net/http"
)

type PermanentHTTPError struct {
	StatusCode int
}

func (e *PermanentHTTPError) Error() string { return fmt.Sprintf("permanent HTTP %d", e.StatusCode) }

func (e *PermanentHTTPError) Code() int    { return e.StatusCode }
func (e *PermanentHTTPError) IsPermanent() bool {
	switch e.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusNotFound, http.StatusGone:
		return true
	}
	return false
}

type RetryableHTTPError struct {
	StatusCode int
}

func (e *RetryableHTTPError) Error() string { return fmt.Sprintf("retryable HTTP %d", e.StatusCode) }

func (e *RetryableHTTPError) Code() int { return e.StatusCode }
func (e *RetryableHTTPError) IsPermanent() bool { return false }