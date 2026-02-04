package retry

import (
	"net/http"
	"strings"
)

// ClassifyClaude classifies errors from Claude CLI
func ClassifyClaude(err error) ErrorType {
	if err == nil {
		return Permanent // No error, shouldn't happen but be safe
	}

	errStr := strings.ToLower(err.Error())

	// Rate limiting
	if strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "too many requests") {
		return RateLimited
	}

	// Timeouts are retryable
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "timed out") ||
		strings.Contains(errStr, "deadline exceeded") {
		return Retryable
	}

	// Network errors are retryable
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "temporary failure") ||
		strings.Contains(errStr, "i/o timeout") {
		return Retryable
	}

	// Server errors from Claude API
	if strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "internal server error") ||
		strings.Contains(errStr, "bad gateway") ||
		strings.Contains(errStr, "service unavailable") {
		return Retryable
	}

	// Overloaded
	if strings.Contains(errStr, "overloaded") ||
		strings.Contains(errStr, "capacity") {
		return RateLimited
	}

	// Everything else is permanent
	return Permanent
}

// ClassifyHTTP classifies HTTP errors by status code
func ClassifyHTTP(statusCode int) ErrorType {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return RateLimited
	case statusCode == http.StatusRequestTimeout,
		statusCode == http.StatusGatewayTimeout:
		return Retryable
	case statusCode >= 500 && statusCode < 600:
		// Server errors are generally retryable
		return Retryable
	case statusCode >= 400 && statusCode < 500:
		// Client errors (except 429) are permanent
		return Permanent
	default:
		return Permanent
	}
}

// ClassifyHTTPError wraps ClassifyHTTP for use with error strings containing status codes
func ClassifyHTTPError(err error) ErrorType {
	if err == nil {
		return Permanent
	}

	errStr := err.Error()

	// Check for common HTTP status patterns
	statusPatterns := map[string]ErrorType{
		"429": RateLimited,
		"408": Retryable,
		"500": Retryable,
		"502": Retryable,
		"503": Retryable,
		"504": Retryable,
		"400": Permanent,
		"401": Permanent,
		"403": Permanent,
		"404": Permanent,
		"422": Permanent,
	}

	for pattern, errType := range statusPatterns {
		if strings.Contains(errStr, pattern) {
			return errType
		}
	}

	// Check for rate limit text
	errLower := strings.ToLower(errStr)
	if strings.Contains(errLower, "rate limit") ||
		strings.Contains(errLower, "too many requests") {
		return RateLimited
	}

	// Check for network errors
	if strings.Contains(errLower, "connection refused") ||
		strings.Contains(errLower, "connection reset") ||
		strings.Contains(errLower, "timeout") ||
		strings.Contains(errLower, "network") {
		return Retryable
	}

	return Permanent
}
