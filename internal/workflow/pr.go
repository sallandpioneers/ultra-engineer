package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/sandbox"
	"github.com/anthropics/ultra-engineer/internal/state"
)

// PRPhase handles the PR creation and merge phase
type PRPhase struct {
	provider providers.Provider
}

// NewPRPhase creates a new PR phase handler
func NewPRPhase(provider providers.Provider) *PRPhase {
	return &PRPhase{
		provider: provider,
	}
}

// PRResult represents the result of PR operations
type PRResult struct {
	PR     *providers.PR
	Merged bool
}

// CreatePR creates a pull request from the implementation
func (p *PRPhase) CreatePR(ctx context.Context, repo string, issue *providers.Issue, plan string, sb *sandbox.Sandbox, baseBranch string) (*PRResult, error) {
	// Check context before each operation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Commit any remaining changes
	commitMsg := fmt.Sprintf("Implement: %s\n\nCloses #%d\n\nImplemented by Ultra Engineer", issue.Title, issue.Number)
	if err := sb.Commit(ctx, commitMsg); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	// Check context before push
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Push the branch
	if err := sb.Push(ctx); err != nil {
		return nil, fmt.Errorf("failed to push: %w", err)
	}

	// Check context before creating PR
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Create PR
	prBody := p.formatPRBody(issue, plan)
	pr, err := p.provider.CreatePR(ctx, repo, providers.PRCreate{
		Title:   fmt.Sprintf("Implement: %s", issue.Title),
		Body:    prBody,
		Head:    sb.BranchName,
		Base:    baseBranch,
		IssueID: issue.Number,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	return &PRResult{PR: pr}, nil
}

// formatPRBody creates a PR description
func (p *PRPhase) formatPRBody(issue *providers.Issue, plan string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Summary\n\nImplements #%d\n\n", issue.Number))
	sb.WriteString("## Implementation Plan\n\n")
	sb.WriteString("<details>\n<summary>Click to expand plan</summary>\n\n")
	sb.WriteString(plan)
	sb.WriteString("\n\n</details>\n\n")
	sb.WriteString("---\n")
	sb.WriteString("*Automated implementation by Ultra Engineer*\n")
	return sb.String()
}

// WaitForMergeable polls until the PR is mergeable or times out
func (p *PRPhase) WaitForMergeable(ctx context.Context, repo string, prNumber int, timeout time.Duration, pollInterval time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		mergeable, err := p.provider.IsMergeable(ctx, repo, prNumber)
		if err != nil {
			return false, fmt.Errorf("failed to check mergeable status: %w", err)
		}

		if mergeable {
			return true, nil
		}

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(pollInterval):
			continue
		}
	}

	return false, nil
}

// Merge merges the PR
func (p *PRPhase) Merge(ctx context.Context, repo string, prNumber int) error {
	return p.provider.MergePR(ctx, repo, prNumber)
}

// GetPR gets the current PR status
func (p *PRPhase) GetPR(ctx context.Context, repo string, prNumber int) (*providers.PR, error) {
	return p.provider.GetPR(ctx, repo, prNumber)
}

// GetPRComments gets comments on the PR
func (p *PRPhase) GetPRComments(ctx context.Context, repo string, prNumber int) ([]*providers.Comment, error) {
	return p.provider.GetPRComments(ctx, repo, prNumber)
}

// PostComment posts a comment on the PR
func (p *PRPhase) PostComment(ctx context.Context, repo string, prNumber int, body string) error {
	return p.provider.CreateComment(ctx, repo, prNumber, body)
}

// PushFix commits and pushes a fix
func (p *PRPhase) PushFix(ctx context.Context, sb *sandbox.Sandbox, message string) error {
	// Check context before commit
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := sb.Commit(ctx, message); err != nil {
		return fmt.Errorf("failed to commit fix: %w", err)
	}

	// Check context before push
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return sb.Push(ctx)
}

// CheckResult represents the result of checking PR status
type CheckResult struct {
	IsMergeable bool
	CIFailed    bool
	CIOutput    string
	HasFeedback bool
	Feedback    string
}

// CheckPRStatus checks the current status of the PR including CI and comments
func (p *PRPhase) CheckPRStatus(ctx context.Context, repo string, prNumber int, lastCommentID int64) (*CheckResult, error) {
	result := &CheckResult{}

	// Get PR status
	pr, err := p.provider.GetPR(ctx, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR: %w", err)
	}

	result.IsMergeable = pr.Mergeable

	// Get comments to check for feedback
	comments, err := p.provider.GetPRComments(ctx, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR comments: %w", err)
	}

	// Find new comments since last check
	for _, comment := range comments {
		if comment.ID > lastCommentID {
			// Skip our own comments (contain state marker)
			if state.ContainsState(comment.Body) {
				continue
			}

			// Check for abort
			if IsAbort(comment.Body) {
				result.HasFeedback = true
				result.Feedback = "/abort"
				return result, nil
			}

			// Collect feedback
			if result.Feedback != "" {
				result.Feedback += "\n\n"
			}
			result.Feedback += comment.Body
			result.HasFeedback = true
		}
	}

	// TODO: Check CI status - this varies by provider
	// For now, we rely on mergeable status which typically includes CI

	return result, nil
}
