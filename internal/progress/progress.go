package progress

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/state"
)

// Status messages with emojis
const (
	StatusAnalyzing       = "🔍 Analyzing issue and generating questions..."
	StatusPlanning        = "📝 Creating implementation plan..."
	StatusPlanReview      = "🔄 Reviewing plan (%d/%d)..."
	StatusWaitingApproval = "⏳ Waiting for approval..."
	StatusImplementing    = "🔨 Implementing changes..."
	StatusCodeReview      = "✅ Code review (%d/%d)..."
	StatusCreatingPR      = "🚀 Creating PR..."
	StatusCompleted       = "✨ Completed successfully"
	StatusCompletedWithPR = "✨ Completed successfully - PR #%d"
	StatusFailed          = "❌ Failed: %s"
)

// Reporter handles posting and updating progress comments on issues
type Reporter struct {
	provider         providers.Provider
	repo             string
	issueNumber      int
	statusCommentID  int64         // ID of the status comment (0 if not created)
	lastUpdate       time.Time     // Time of last update
	debounceInterval time.Duration // Minimum time between updates
	mu               sync.Mutex
	enabled          bool
}

// NewReporter creates a new progress reporter
func NewReporter(provider providers.Provider, repo string, issueNumber int, debounceInterval time.Duration, enabled bool) *Reporter {
	return &Reporter{
		provider:         provider,
		repo:             repo,
		issueNumber:      issueNumber,
		debounceInterval: debounceInterval,
		enabled:          enabled,
	}
}

// Update posts or updates the status comment with debouncing
// Updates are skipped if less than debounceInterval has passed since the last update
func (r *Reporter) Update(ctx context.Context, status string) error {
	if !r.enabled {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check debounce
	if time.Since(r.lastUpdate) < r.debounceInterval && r.statusCommentID != 0 {
		return nil // Skip this update
	}

	return r.doUpdate(ctx, status)
}

// ForceUpdate posts or updates the status comment, bypassing debounce
func (r *Reporter) ForceUpdate(ctx context.Context, status string) error {
	if !r.enabled {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.doUpdate(ctx, status)
}

// Finalize posts the final status update (always posted, no debounce)
func (r *Reporter) Finalize(ctx context.Context, status string) error {
	if !r.enabled {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.doUpdate(ctx, status)
}

// doUpdate performs the actual update (must be called with lock held)
func (r *Reporter) doUpdate(ctx context.Context, status string) error {
	body := formatStatusComment(status)

	if r.statusCommentID == 0 {
		// Create new comment
		commentID, err := r.provider.CreateComment(ctx, r.repo, r.issueNumber, body)
		if err != nil {
			return fmt.Errorf("failed to create status comment: %w", err)
		}
		r.statusCommentID = commentID
	} else {
		// Update existing comment
		if err := r.provider.UpdateComment(ctx, r.repo, r.statusCommentID, body); err != nil {
			return fmt.Errorf("failed to update status comment: %w", err)
		}
	}

	r.lastUpdate = time.Now()
	return nil
}

// formatStatusComment formats the status message for display
func formatStatusComment(status string) string {
	return state.AddBotMarker(fmt.Sprintf("**Status:** %s", status))
}

// Helpers for formatting status messages

// FormatPlanReview formats the plan review status message
func FormatPlanReview(iteration, total int) string {
	return fmt.Sprintf(StatusPlanReview, iteration, total)
}

// FormatCodeReview formats the code review status message
func FormatCodeReview(iteration, total int) string {
	return fmt.Sprintf(StatusCodeReview, iteration, total)
}

// FormatCompleted formats the completed status message with optional PR number
func FormatCompleted(prNumber int) string {
	if prNumber > 0 {
		return fmt.Sprintf(StatusCompletedWithPR, prNumber)
	}
	return StatusCompleted
}

// FormatFailed formats the failed status message with error
func FormatFailed(err error) string {
	return fmt.Sprintf(StatusFailed, err.Error())
}
