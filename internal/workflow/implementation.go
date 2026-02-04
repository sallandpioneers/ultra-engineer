package workflow

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/sandbox"
)

// MergeConflictMarker is the marker Claude outputs when it cannot resolve a conflict
const MergeConflictMarker = "MERGE_CONFLICT_UNRESOLVED:"

// ParseMergeConflictMarker extracts conflicting files from Claude's output
func ParseMergeConflictMarker(output string) []string {
	re := regexp.MustCompile(`MERGE_CONFLICT_UNRESOLVED:\s*(.+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		return nil
	}

	files := strings.Split(matches[1], ",")
	var result []string
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

// HasGitError checks if the output contains common git error patterns
func HasGitError(output string) bool {
	errorPatterns := []string{
		"push rejected",
		"failed to push",
		"CONFLICT",
		"rebase failed",
		"fatal: ",
		"error: failed to",
	}

	outputLower := strings.ToLower(output)
	for _, pattern := range errorPatterns {
		if strings.Contains(outputLower, strings.ToLower(pattern)) {
			// Check it's not in a resolved context
			if !strings.Contains(outputLower, "resolved") && !strings.Contains(outputLower, "successfully") {
				return true
			}
		}
	}
	return false
}

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

// Implement executes the implementation plan (without git operations)
func (i *ImplementationPhase) Implement(ctx context.Context, issueTitle string, sb *sandbox.Sandbox) error {
	prompt := fmt.Sprintf(claude.Prompts.Implement, issueTitle)

	_, _, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep"},
	})
	return err
}

// ImplementResult contains the result of implementation with git operations
type ImplementResult struct {
	Success              bool
	MergeConflict        bool
	ConflictingFiles     []string
	Output               string
}

// ImplementWithGit executes the implementation plan and handles git commit/push
func (i *ImplementationPhase) ImplementWithGit(ctx context.Context, issueTitle string, issueNum int, branchName string, sb *sandbox.Sandbox) (*ImplementResult, error) {
	prompt := fmt.Sprintf(claude.Prompts.ImplementGit, issueTitle, branchName, issueTitle, issueNum, branchName)

	output, _, err := i.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      sb.RepoDir,
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep"},
	})

	result := &ImplementResult{
		Output: output,
	}

	// Check for merge conflict marker
	if conflictFiles := ParseMergeConflictMarker(output); len(conflictFiles) > 0 {
		result.MergeConflict = true
		result.ConflictingFiles = conflictFiles
		return result, nil
	}

	// Check for other git error patterns
	if HasGitError(output) {
		return result, fmt.Errorf("git operation failed: check output for details")
	}

	if err != nil {
		return result, err
	}

	result.Success = true
	return result, nil
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
