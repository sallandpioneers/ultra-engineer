package progress

import (
	"context"
	"fmt"
	"strings"
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
	StatusWaitingAnswers  = "❓ Waiting for answers..."
	StatusWaitingApproval = "⏳ Waiting for approval..."
	StatusImplementing    = "🔨 Implementing changes..."
	StatusCodeReview      = "✅ Code review (%d/%d)..."
	StatusCreatingPR      = "🚀 Creating PR..."
	StatusCompleted       = "✨ Completed successfully"
	StatusCompletedWithPR = "✨ Completed successfully - PR #%d"
	StatusFailed          = "❌ Failed: %s"

	// CI status messages
	StatusWaitingCI        = "⏳ Waiting for CI to complete..."
	StatusCISuccess        = "✅ CI passed"
	StatusCIFailed         = "❌ CI failed: %s"
	StatusFixingCI         = "🔧 Fixing CI failure (attempt %d/%d)..."
	StatusCITimeout        = "⏰ CI timed out after %s"
	StatusCIFixMaxAttempts = "❌ CI fix attempts exhausted (%d/%d)"

	// PR merge status messages
	StatusWaitingPRApproval = "⏳ Waiting for PR approval..."
	StatusMerged            = "🎉 PR merged successfully"
)

// Reporter handles posting and updating progress comments on issues
type Reporter struct {
	provider         providers.Provider
	repo             string
	issueNumber      int
	statusCommentID  int64         // ID of the status comment (0 if not created)
	lastUpdate       time.Time     // Time of last update
	lastStatusMsg    string        // Last status message (to avoid duplicate updates)
	debounceInterval time.Duration // Minimum time between updates
	mu               sync.Mutex
	enabled          bool
	st               *state.State // State to persist with status updates (includes history)
}

// NewReporter creates a new progress reporter (without state persistence)
func NewReporter(provider providers.Provider, repo string, issueNumber int, debounceInterval time.Duration, enabled bool) *Reporter {
	return &Reporter{
		provider:         provider,
		repo:             repo,
		issueNumber:      issueNumber,
		debounceInterval: debounceInterval,
		enabled:          enabled,
	}
}

// NewReporterWithState creates a reporter that uses state for persistence
func NewReporterWithState(provider providers.Provider, repo string, issueNumber int, debounceInterval time.Duration, enabled bool, st *state.State) *Reporter {
	r := &Reporter{
		provider:         provider,
		repo:             repo,
		issueNumber:      issueNumber,
		statusCommentID:  st.StatusCommentID,
		debounceInterval: debounceInterval,
		enabled:          enabled,
		st:               st,
	}

	// Load last status message from history to avoid duplicate updates
	if len(st.StatusHistory) > 0 {
		lastEntry := st.StatusHistory[len(st.StatusHistory)-1]
		parts := strings.SplitN(lastEntry, "|", 2)
		if len(parts) == 2 {
			r.lastStatusMsg = parts[1]
		}
	}

	return r
}

// Update posts or updates the status comment with debouncing
// Updates are skipped if less than debounceInterval has passed since the last update
func (r *Reporter) Update(ctx context.Context, status string) error {
	if !r.enabled {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Skip if status hasn't changed (avoid duplicate updates for polling)
	if status == r.lastStatusMsg {
		return nil
	}

	// Check debounce
	if time.Since(r.lastUpdate) < r.debounceInterval && r.statusCommentID != 0 {
		return nil // Skip this update
	}

	return r.doUpdate(ctx, status)
}

// ForceUpdate posts or updates the status comment, bypassing debounce
// If status hasn't changed, still persists state but doesn't add duplicate log entry
func (r *Reporter) ForceUpdate(ctx context.Context, status string) error {
	if !r.enabled {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// If status hasn't changed, persist state without adding duplicate log entry
	if status == r.lastStatusMsg {
		return r.persistStateOnly(ctx)
	}

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
	// Add to history (stored in state for persistence)
	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("%s|%s", timestamp, status)
	if r.st != nil {
		r.st.StatusHistory = append(r.st.StatusHistory, entry)
	}

	// Track last status to avoid duplicate updates
	r.lastStatusMsg = status

	body := r.formatStatusLog()

	if r.statusCommentID == 0 {
		// Create new comment
		commentID, err := r.provider.CreateComment(ctx, r.repo, r.issueNumber, body)
		if err != nil {
			return fmt.Errorf("failed to create status comment: %w", err)
		}
		r.statusCommentID = commentID
		// Update state so it persists across daemon restarts
		if r.st != nil {
			r.st.StatusCommentID = commentID
		}
	} else {
		// Update existing comment
		if err := r.provider.UpdateComment(ctx, r.repo, r.statusCommentID, body); err != nil {
			return fmt.Errorf("failed to update status comment: %w", err)
		}
	}

	r.lastUpdate = time.Now()
	return nil
}

// persistStateOnly updates the comment to persist state changes without adding a new status entry
// Must be called with lock held
func (r *Reporter) persistStateOnly(ctx context.Context) error {
	if r.statusCommentID == 0 || r.st == nil {
		return nil // Nothing to persist
	}

	body := r.formatStatusLog()
	if err := r.provider.UpdateComment(ctx, r.repo, r.statusCommentID, body); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	r.lastUpdate = time.Now()
	return nil
}

// formatStatusLog formats all status entries as a log with timestamps
func (r *Reporter) formatStatusLog() string {
	var lines []string
	lines = append(lines, "**Progress Log**")
	lines = append(lines, "")

	// Use history from state if available
	if r.st != nil {
		for _, entry := range r.st.StatusHistory {
			// Entry format: "HH:MM:SS|message"
			parts := strings.SplitN(entry, "|", 2)
			if len(parts) == 2 {
				lines = append(lines, fmt.Sprintf("`%s` %s", parts[0], parts[1]))
			}
		}
	}

	body := joinLines(lines)

	// Include state in the comment if available
	if r.st != nil {
		stateStr, err := r.st.Serialize()
		if err == nil {
			body = body + "\n\n" + stateStr
		}
	}

	return state.AddBotMarker(body)
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
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

// FormatCIFailed formats the CI failed status message
func FormatCIFailed(checkName string) string {
	return fmt.Sprintf(StatusCIFailed, checkName)
}

// FormatFixingCI formats the fixing CI status message
func FormatFixingCI(attempt, maxAttempts int) string {
	return fmt.Sprintf(StatusFixingCI, attempt, maxAttempts)
}

// FormatCITimeout formats the CI timeout status message
func FormatCITimeout(duration time.Duration) string {
	return fmt.Sprintf(StatusCITimeout, duration)
}

// FormatCIFixMaxAttempts formats the max attempts reached status message
func FormatCIFixMaxAttempts(attempts, maxAttempts int) string {
	return fmt.Sprintf(StatusCIFixMaxAttempts, attempts, maxAttempts)
}
