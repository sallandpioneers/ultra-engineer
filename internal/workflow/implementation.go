package workflow

import (
	"context"
	"fmt"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/sandbox"
	"github.com/anthropics/ultra-engineer/internal/state"
)

// ImplementationPhase handles the implementation phase of issue processing
type ImplementationPhase struct {
	claude       *claude.Client
	provider     providers.Provider
	reviewCycles int
}

// NewImplementationPhase creates a new implementation phase handler
func NewImplementationPhase(claudeClient *claude.Client, provider providers.Provider, reviewCycles int) *ImplementationPhase {
	return &ImplementationPhase{
		claude:       claudeClient,
		provider:     provider,
		reviewCycles: reviewCycles,
	}
}

// ImplementResult represents the result of implementation
type ImplementResult struct {
	Summary   string
	SessionID string
}

// Implement executes the implementation plan
func (i *ImplementationPhase) Implement(ctx context.Context, issue *providers.Issue, plan string, st *state.State, sb *sandbox.Sandbox) (*ImplementResult, error) {
	prompt := fmt.Sprintf(claude.Prompts.Implement,
		issue.Title,
		plan,
	)

	// Allow Claude to use file editing and bash tools
	allowedTools := []string{
		"Read",
		"Write",
		"Edit",
		"Bash",
		"Glob",
		"Grep",
	}

	response, sessionID, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		SessionID:    st.SessionID,
		Prompt:       prompt,
		AllowedTools: allowedTools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to implement: %w", err)
	}

	return &ImplementResult{
		Summary:   response,
		SessionID: sessionID,
	}, nil
}

// ReviewCode runs a single code review iteration
func (i *ImplementationPhase) ReviewCode(ctx context.Context, iteration int, st *state.State, sb *sandbox.Sandbox) (*ImplementResult, error) {
	prompt := fmt.Sprintf(claude.Prompts.ReviewCode,
		iteration,
		iteration,
	)

	allowedTools := []string{
		"Read",
		"Write",
		"Edit",
		"Bash",
		"Glob",
		"Grep",
	}

	response, sessionID, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		SessionID:    st.SessionID,
		Prompt:       prompt,
		AllowedTools: allowedTools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to review code (iteration %d): %w", iteration, err)
	}

	return &ImplementResult{
		Summary:   response,
		SessionID: sessionID,
	}, nil
}

// RunFullCodeReviewCycle runs all code review iterations
func (i *ImplementationPhase) RunFullCodeReviewCycle(ctx context.Context, st *state.State, sb *sandbox.Sandbox, progressCallback func(iteration int)) (*ImplementResult, error) {
	var lastSummary string
	var lastSessionID string

	for iter := 1; iter <= i.reviewCycles; iter++ {
		// Check for context cancellation before each iteration
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if progressCallback != nil {
			progressCallback(iter)
		}

		result, err := i.ReviewCode(ctx, iter, st, sb)
		if err != nil {
			return nil, err
		}

		lastSummary = result.Summary
		lastSessionID = result.SessionID
		st.SessionID = lastSessionID
	}

	return &ImplementResult{
		Summary:   lastSummary,
		SessionID: lastSessionID,
	}, nil
}

// FixCIFailure attempts to fix CI failures
func (i *ImplementationPhase) FixCIFailure(ctx context.Context, ciOutput string, st *state.State, sb *sandbox.Sandbox) (*ImplementResult, error) {
	prompt := fmt.Sprintf(claude.Prompts.FixCI, ciOutput)

	allowedTools := []string{
		"Read",
		"Write",
		"Edit",
		"Bash",
		"Glob",
		"Grep",
	}

	response, sessionID, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		SessionID:    st.SessionID,
		Prompt:       prompt,
		AllowedTools: allowedTools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fix CI: %w", err)
	}

	return &ImplementResult{
		Summary:   response,
		SessionID: sessionID,
	}, nil
}

// AddressFeedback addresses user feedback on the implementation
func (i *ImplementationPhase) AddressFeedback(ctx context.Context, feedback string, implementationSummary string, st *state.State, sb *sandbox.Sandbox) (*ImplementResult, error) {
	prompt := fmt.Sprintf(claude.Prompts.AddressFeedback,
		feedback,
		implementationSummary,
	)

	allowedTools := []string{
		"Read",
		"Write",
		"Edit",
		"Bash",
		"Glob",
		"Grep",
	}

	response, sessionID, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		SessionID:    st.SessionID,
		Prompt:       prompt,
		AllowedTools: allowedTools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to address feedback: %w", err)
	}

	return &ImplementResult{
		Summary:   response,
		SessionID: sessionID,
	}, nil
}

// PostProgress posts implementation progress as a comment
func (i *ImplementationPhase) PostProgress(ctx context.Context, repo string, issueNum int, message string, st *state.State) error {
	commentWithState, err := st.AppendToBody(message)
	if err != nil {
		return fmt.Errorf("failed to append state: %w", err)
	}

	return i.provider.CreateComment(ctx, repo, issueNum, commentWithState)
}
