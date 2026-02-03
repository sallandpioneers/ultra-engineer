package workflow

import (
	"context"
	"fmt"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/sandbox"
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

// Implement executes the implementation plan
func (i *ImplementationPhase) Implement(ctx context.Context, issueTitle string, sb *sandbox.Sandbox) error {
	prompt := fmt.Sprintf(claude.Prompts.Implement, issueTitle)

	_, _, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep"},
	})
	return err
}

// ReviewCode runs a single code review iteration
func (i *ImplementationPhase) ReviewCode(ctx context.Context, iteration int, sb *sandbox.Sandbox) error {
	prompt := fmt.Sprintf(claude.Prompts.ReviewCode, iteration)

	_, _, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep"},
	})
	return err
}

// RunFullCodeReviewCycle runs all code review iterations
func (i *ImplementationPhase) RunFullCodeReviewCycle(ctx context.Context, sb *sandbox.Sandbox, progressCallback func(iteration int)) error {
	for iter := 1; iter <= i.reviewCycles; iter++ {
		if progressCallback != nil {
			progressCallback(iter)
		}
		if err := i.ReviewCode(ctx, iter, sb); err != nil {
			return err
		}
	}
	return nil
}

// FixCIFailure attempts to fix CI failures
func (i *ImplementationPhase) FixCIFailure(ctx context.Context, ciOutput string, sb *sandbox.Sandbox) error {
	prompt := fmt.Sprintf(claude.Prompts.FixCI, ciOutput)

	_, _, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep"},
	})
	return err
}

// AddressFeedback addresses user feedback on the implementation
func (i *ImplementationPhase) AddressFeedback(ctx context.Context, feedback string, sb *sandbox.Sandbox) error {
	prompt := fmt.Sprintf(`Address this feedback on the implementation:

%s

Read .ultra-engineer/plan.md for context. Fix any issues in the code.
Output "FEEDBACK_ADDRESSED" when done.`, feedback)

	_, _, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep"},
	})
	return err
}
