package retry

import (
	"errors"
	"net/http"
	"testing"
)

func TestClassifyClaude(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorType
	}{
		// Rate limiting
		{"rate limit", errors.New("rate limit exceeded"), RateLimited},
		{"429 error", errors.New("HTTP 429: too many requests"), RateLimited},
		{"too many requests", errors.New("too many requests"), RateLimited},
		{"overloaded", errors.New("API overloaded"), RateLimited},
		{"capacity", errors.New("at capacity"), RateLimited},

		// Timeouts
		{"timeout", errors.New("connection timeout"), Retryable},
		{"timed out", errors.New("request timed out"), Retryable},
		{"deadline exceeded", errors.New("context deadline exceeded"), Retryable},

		// Network errors
		{"connection refused", errors.New("connection refused"), Retryable},
		{"connection reset", errors.New("connection reset by peer"), Retryable},
		{"network error", errors.New("network unreachable"), Retryable},
		{"i/o timeout", errors.New("i/o timeout"), Retryable},

		// Server errors
		{"500 error", errors.New("HTTP 500: internal server error"), Retryable},
		{"502 error", errors.New("HTTP 502 bad gateway"), Retryable},
		{"503 error", errors.New("HTTP 503 service unavailable"), Retryable},
		{"504 error", errors.New("HTTP 504 gateway timeout"), Retryable},

		// Permanent errors
		{"auth error", errors.New("authentication failed"), Permanent},
		{"invalid request", errors.New("invalid request body"), Permanent},
		{"not found", errors.New("resource not found"), Permanent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyClaude(tt.err)
			if result != tt.expected {
				t.Errorf("ClassifyClaude(%q) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestClassifyHTTP(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   ErrorType
	}{
		// Rate limited
		{http.StatusTooManyRequests, RateLimited},

		// Retryable
		{http.StatusRequestTimeout, Retryable},
		{http.StatusGatewayTimeout, Retryable},
		{http.StatusInternalServerError, Retryable},
		{http.StatusBadGateway, Retryable},
		{http.StatusServiceUnavailable, Retryable},

		// Permanent
		{http.StatusBadRequest, Permanent},
		{http.StatusUnauthorized, Permanent},
		{http.StatusForbidden, Permanent},
		{http.StatusNotFound, Permanent},
		{http.StatusConflict, Permanent},

		// Success (shouldn't be classified, but handle gracefully)
		{http.StatusOK, Permanent},
		{http.StatusCreated, Permanent},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := ClassifyHTTP(tt.statusCode)
			if result != tt.expected {
				t.Errorf("ClassifyHTTP(%d) = %v, want %v", tt.statusCode, result, tt.expected)
			}
		})
	}
}

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorType
	}{
		{"429 in error", errors.New("API error 429: rate limited"), RateLimited},
		{"500 in error", errors.New("API error 500: internal error"), Retryable},
		{"502 in error", errors.New("API error 502: bad gateway"), Retryable},
		{"503 in error", errors.New("API error 503: unavailable"), Retryable},
		{"400 in error", errors.New("API error 400: bad request"), Permanent},
		{"401 in error", errors.New("API error 401: unauthorized"), Permanent},
		{"404 in error", errors.New("API error 404: not found"), Permanent},
		{"rate limit text", errors.New("rate limit exceeded"), RateLimited},
		{"connection refused", errors.New("connection refused"), Retryable},
		{"timeout", errors.New("request timeout"), Retryable},
		{"unknown error", errors.New("something went wrong"), Permanent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyHTTPError(tt.err)
			if result != tt.expected {
				t.Errorf("ClassifyHTTPError(%q) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
