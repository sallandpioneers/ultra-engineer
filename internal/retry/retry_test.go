package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCalculateBackoff(t *testing.T) {
	base := 100 * time.Millisecond

	// Test exponential growth
	tests := []struct {
		attempt     int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{0, 100 * time.Millisecond, 125 * time.Millisecond},  // 100ms + 0-25% jitter
		{1, 200 * time.Millisecond, 250 * time.Millisecond},  // 200ms + 0-25% jitter
		{2, 400 * time.Millisecond, 500 * time.Millisecond},  // 400ms + 0-25% jitter
		{3, 800 * time.Millisecond, 1000 * time.Millisecond}, // 800ms + 0-25% jitter
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			backoff := calculateBackoff(base, tt.attempt)
			if backoff < tt.minExpected || backoff > tt.maxExpected {
				t.Errorf("attempt %d: got %v, want between %v and %v", tt.attempt, backoff, tt.minExpected, tt.maxExpected)
			}
		})
	}
}

func TestDo_Success(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    3,
		BackoffBase:    10 * time.Millisecond,
		RateLimitRetry: 50 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	calls := 0
	err := Do(ctx, opts, func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestDo_RetryThenSuccess(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    3,
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	calls := 0
	err := Do(ctx, opts, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestDo_PermanentError(t *testing.T) {
	ctx := context.Background()
	permanentErr := errors.New("permanent error")
	opts := Options{
		MaxAttempts:    3,
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Permanent },
	}

	calls := 0
	err := Do(ctx, opts, func() error {
		calls++
		return permanentErr
	})

	if err != permanentErr {
		t.Errorf("expected permanent error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry for permanent), got %d", calls)
	}
}

func TestDo_MaxAttempts(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    3,
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	calls := 0
	expectedErr := errors.New("always fails")
	err := Do(ctx, opts, func() error {
		calls++
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	opts := Options{
		MaxAttempts:    10,
		BackoffBase:    100 * time.Millisecond,
		RateLimitRetry: 500 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	calls := 0
	err := Do(ctx, opts, func() error {
		calls++
		if calls == 2 {
			cancel()
		}
		return errors.New("keep retrying")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestDo_RateLimited(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    3,
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 10 * time.Millisecond,
		Classifier: func(err error) ErrorType {
			if err.Error() == "rate limited" {
				return RateLimited
			}
			return Permanent
		},
	}

	calls := 0
	start := time.Now()
	err := Do(ctx, opts, func() error {
		calls++
		if calls < 2 {
			return errors.New("rate limited")
		}
		return nil
	})

	elapsed := time.Since(start)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
	// Should have waited at least the rate limit retry duration
	if elapsed < 10*time.Millisecond {
		t.Errorf("expected at least 10ms delay for rate limit, got %v", elapsed)
	}
}

func TestDoWithResult_Success(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    3,
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	result, err := DoWithResult(ctx, opts, func() (string, error) {
		return "hello", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestDoWithResult_RetryThenSuccess(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    3,
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	calls := 0
	result, err := DoWithResult(ctx, opts, func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("transient")
		}
		return 42, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestDo_InfiniteRetry_StopsOnPermanent(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    0, // infinite
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier: func(err error) ErrorType {
			if err.Error() == "permanent" {
				return Permanent
			}
			return Retryable
		},
	}

	calls := 0
	err := Do(ctx, opts, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return errors.New("permanent")
	})

	if err == nil || err.Error() != "permanent" {
		t.Errorf("expected permanent error, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls (2 transient + 1 permanent), got %d", calls)
	}
}

func TestDo_InfiniteRetry_RespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := Options{
		MaxAttempts:    0, // infinite
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	calls := 0
	go func() {
		// Cancel after a short delay to allow some retries
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := Do(ctx, opts, func() error {
		calls++
		return errors.New("keep retrying")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if calls < 1 {
		t.Errorf("expected at least 1 call, got %d", calls)
	}
}

func TestDo_InfiniteRetry_EventualSuccess(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    0, // infinite
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	calls := 0
	err := Do(ctx, opts, func() error {
		calls++
		if calls < 5 {
			return errors.New("transient")
		}
		return nil // success on 5th attempt
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if calls != 5 {
		t.Errorf("expected 5 calls, got %d", calls)
	}
}

func TestDoWithResult_InfiniteRetry(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    0, // infinite
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	calls := 0
	result, err := DoWithResult(ctx, opts, func() (int, error) {
		calls++
		if calls < 4 {
			return 0, errors.New("transient")
		}
		return 42, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
	if calls != 4 {
		t.Errorf("expected 4 calls, got %d", calls)
	}
}

func TestDo_InfiniteRetry_NegativeMaxAttempts(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		MaxAttempts:    -1, // negative also means infinite
		BackoffBase:    1 * time.Millisecond,
		RateLimitRetry: 5 * time.Millisecond,
		Classifier:     func(err error) ErrorType { return Retryable },
	}

	calls := 0
	err := Do(ctx, opts, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}
