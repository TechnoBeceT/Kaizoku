package util

import (
	"context"
	"errors"
	"strings"

	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
)

// Error category constants for source event logging.
const (
	ErrCatNetwork     = "network"
	ErrCatTimeout     = "timeout"
	ErrCatRateLimit   = "rate_limit"
	ErrCatServerError = "server_error"
	ErrCatNotFound    = "not_found"
	ErrCatParse       = "parse"
	ErrCatCancelled   = "cancelled"
	ErrCatUnknown     = "unknown"
)

// CategorizeError inspects an error and returns a category string for reporting.
func CategorizeError(err error) string {
	if err == nil {
		return ""
	}

	// Context errors
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrCatTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrCatCancelled
	}

	// Suwayomi-specific errors
	if errors.Is(err, suwayomi.ErrNotFound) {
		return ErrCatNotFound
	}

	msg := err.Error()

	// HTTP status patterns
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests") {
		return ErrCatRateLimit
	}
	if strings.Contains(msg, "404") || strings.Contains(msg, "not found") {
		return ErrCatNotFound
	}
	if strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "internal server error") {
		return ErrCatServerError
	}

	// Network / connection errors
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "dial tcp") || strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "connection reset") || strings.Contains(msg, "broken pipe") {
		return ErrCatNetwork
	}

	// Timeout patterns
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") {
		return ErrCatTimeout
	}

	// Parse errors
	if strings.Contains(msg, "unmarshal") || strings.Contains(msg, "json:") || strings.Contains(msg, "invalid character") ||
		strings.Contains(msg, "unexpected end") || strings.Contains(msg, "decode") {
		return ErrCatParse
	}

	return ErrCatUnknown
}
