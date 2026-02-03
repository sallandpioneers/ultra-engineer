package workflow

import (
	"context"
	"fmt"
	"log"
)

// ReviewCycleRunner handles running review cycles with progress tracking
type ReviewCycleRunner struct {
	totalCycles int
	logger      *log.Logger
}

// NewReviewCycleRunner creates a new review cycle runner
func NewReviewCycleRunner(totalCycles int, logger *log.Logger) *ReviewCycleRunner {
	return &ReviewCycleRunner{
		totalCycles: totalCycles,
		logger:      logger,
	}
}

// ReviewFunc is a function that performs a single review iteration
type ReviewFunc func(ctx context.Context, iteration int) (string, error)

// ProgressFunc is called to report progress
type ProgressFunc func(iteration, total int, result string)

// Run executes the full review cycle
func (r *ReviewCycleRunner) Run(ctx context.Context, reviewFn ReviewFunc, progressFn ProgressFunc) (string, error) {
	var lastResult string

	for i := 1; i <= r.totalCycles; i++ {
		if r.logger != nil {
			r.logger.Printf("Running review iteration %d/%d", i, r.totalCycles)
		}

		result, err := reviewFn(ctx, i)
		if err != nil {
			return "", fmt.Errorf("review iteration %d failed: %w", i, err)
		}

		lastResult = result

		if progressFn != nil {
			progressFn(i, r.totalCycles, result)
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
	}

	return lastResult, nil
}

// FormatReviewProgress formats a progress message for review cycles
func FormatReviewProgress(phase string, iteration, total int) string {
	return fmt.Sprintf("**%s Review Progress:** %d/%d complete", phase, iteration, total)
}
