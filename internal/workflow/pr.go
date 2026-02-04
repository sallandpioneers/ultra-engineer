package workflow

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/providers"
)

// PRPhase handles the PR creation and merge phase
type PRPhase struct {
	provider providers.Provider
	claude   *claude.Client
}

// NewPRPhase creates a new PR phase handler
func NewPRPhase(provider providers.Provider, claudeClient *claude.Client) *PRPhase {
	return &PRPhase{provider: provider, claude: claudeClient}
}

// PRResult represents the result of PR operations
type PRResult struct {
	PR     *providers.PR
	Merged bool
}

// CreatePR creates a pull request from the implementation
func (p *PRPhase) CreatePR(ctx context.Context, repo string, issue *providers.Issue, headBranch, baseBranch, repoDir string) (*PRResult, error) {
	// Ensure the branch is pushed to remote before creating PR
	if err := p.ensureBranchPushed(repoDir, headBranch); err != nil {
		return nil, fmt.Errorf("failed to push branch: %w", err)
	}

	// Generate summary of changes using Claude
	summary, err := p.GenerateChangeSummary(ctx, repoDir, baseBranch, headBranch)
	if err != nil {
		// Fall back to simple description if summary generation fails
		summary = ""
	}

	prBody := p.formatPRBody(issue, summary)

	pr, err := p.provider.CreatePR(ctx, repo, providers.PRCreate{
		Title:   fmt.Sprintf("Implement: %s", issue.Title),
		Body:    prBody,
		Head:    headBranch,
		Base:    baseBranch,
		IssueID: issue.Number,
	})
	if err != nil {
		// Check if PR already exists for this branch - try to find and return it
		if existingPR := p.findExistingPR(ctx, repo, err); existingPR != nil {
			return &PRResult{PR: existingPR}, nil
		}
		return nil, err
	}

	return &PRResult{PR: pr}, nil
}

// findExistingPR extracts PR number from "already exists" error and returns the PR
func (p *PRPhase) findExistingPR(ctx context.Context, repo string, err error) *providers.PR {
	errStr := err.Error()
	if !strings.Contains(errStr, "already exists") {
		return nil
	}

	// Extract PR number from URL like "https://github.com/owner/repo/pull/123"
	re := regexp.MustCompile(`/pull/(\d+)`)
	matches := re.FindStringSubmatch(errStr)
	if len(matches) < 2 {
		return nil
	}

	prNum, parseErr := strconv.Atoi(matches[1])
	if parseErr != nil {
		return nil
	}

	pr, getErr := p.provider.GetPR(ctx, repo, prNum)
	if getErr != nil {
		return nil
	}

	return pr
}

// ensureBranchPushed ensures the branch is pushed to the remote
// This handles cases where the remote branch was deleted (e.g., after closing a PR)
func (p *PRPhase) ensureBranchPushed(repoDir, branch string) error {
	cmd := exec.Command("git", "push", "-u", "origin", branch, "--force-with-lease")
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func (p *PRPhase) formatPRBody(issue *providers.Issue, summary string) string {
	var sb strings.Builder

	if summary != "" {
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("## Summary\n\nImplements the requested changes.\n\n")
	}

	sb.WriteString(fmt.Sprintf("Closes #%d\n\n", issue.Number))
	sb.WriteString("---\n*Automated by Ultra Engineer*\n")
	return sb.String()
}

// GenerateChangeSummary spawns Claude to analyze the git diff and generate a summary
func (p *PRPhase) GenerateChangeSummary(ctx context.Context, repoDir, baseBranch, headBranch string) (string, error) {
	prompt := fmt.Sprintf(claude.Prompts.SummarizeChanges, baseBranch, headBranch)

	result, err := p.claude.Run(ctx, claude.RunOptions{
		WorkDir: repoDir,
		Prompt:  prompt,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate change summary: %w", err)
	}

	return strings.TrimSpace(result), nil
}
