package retry

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"github.com/anthropics/ultra-engineer/internal/config"
)

// ErrorType classifies errors for retry decisions
type ErrorType int

const (
	// Retryable indicates the error is transient and should be retried
	Retryable ErrorType = iota
	// RateLimited indicates rate limiting - use longer backoff
	RateLimited
	// Permanent indicates the error should not be retried
	Permanent
)

// Classifier is a function that classifies an error
type Classifier func(error) ErrorType

// Options configures retry behavior
type Options struct {
	MaxAttempts    int
	BackoffBase    time.Duration
	RateLimitRetry time.Duration
	Classifier     Classifier
}

// DefaultOptions returns retry options from config
func DefaultOptions(cfg config.RetryConfig) Options {
	return Options{
		MaxAttempts:    cfg.MaxAttempts,
		BackoffBase:    cfg.BackoffBase,
		RateLimitRetry: cfg.RateLimitRetry,
		Classifier:     nil, // Must be set by caller
	}
}

// maxBackoff caps the maximum backoff duration to prevent overflow
const maxBackoff = 5 * time.Minute

// calculateBackoff computes the delay for a given attempt using exponential backoff with jitter
// Formula: delay = base * 2^attempt + jitter(0-25%), capped at maxBackoff
func calculateBackoff(base time.Duration, attempt int) time.Duration {
	// Exponential backoff: base * 2^attempt
	multiplier := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(base) * multiplier)

	// Cap at maximum to prevent overflow
	if delay > maxBackoff {
		delay = maxBackoff
	}

	// Add jitter: 0-25% of the delay (rand/v2 is automatically seeded)
	jitter := time.Duration(rand.Float64() * 0.25 * float64(delay))
	return delay + jitter
}

// Do executes a function with retry logic
// When MaxAttempts <= 0, retries indefinitely (infinite mode)
// The function is retried based on error classification until:
// - Success
// - Context cancellation
// - Permanent error (always stops retries, even in infinite mode)
func Do(ctx context.Context, opts Options, fn func() error) error {
	var lastErr error
	infinite := opts.MaxAttempts <= 0

	for attempt := 0; infinite || attempt < opts.MaxAttempts; attempt++ {
		// Check context before each attempt
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Classify the error
		errType := Permanent
		if opts.Classifier != nil {
			errType = opts.Classifier(lastErr)
		}

		switch errType {
		case Permanent:
			return lastErr
		case RateLimited:
			// Use rate limit retry duration
			if err := sleep(ctx, opts.RateLimitRetry); err != nil {
				return err
			}
		case Retryable:
			// Use exponential backoff (skip delay on last attempt in finite mode)
			if infinite || attempt < opts.MaxAttempts-1 {
				backoff := calculateBackoff(opts.BackoffBase, attempt)
				if err := sleep(ctx, backoff); err != nil {
					return err
				}
			}
		}
	}

	return lastErr
}

// DoWithResult executes a function that returns a value with retry logic
// When MaxAttempts <= 0, retries indefinitely (infinite mode)
func DoWithResult[T any](ctx context.Context, opts Options, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error
	infinite := opts.MaxAttempts <= 0

	for attempt := 0; infinite || attempt < opts.MaxAttempts; attempt++ {
		// Check context before each attempt
		if err := ctx.Err(); err != nil {
			return result, err
		}

		result, lastErr = fn()
		if lastErr == nil {
			return result, nil
		}

		// Classify the error
		errType := Permanent
		if opts.Classifier != nil {
			errType = opts.Classifier(lastErr)
		}

		switch errType {
		case Permanent:
			return result, lastErr
		case RateLimited:
			// Use rate limit retry duration
			if err := sleep(ctx, opts.RateLimitRetry); err != nil {
				return result, err
			}
		case Retryable:
			// Use exponential backoff (skip delay on last attempt in finite mode)
			if infinite || attempt < opts.MaxAttempts-1 {
				backoff := calculateBackoff(opts.BackoffBase, attempt)
				if err := sleep(ctx, backoff); err != nil {
					return result, err
				}
			}
		}
	}

	return result, lastErr
}

// sleep waits for the given duration or until context is cancelled
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
