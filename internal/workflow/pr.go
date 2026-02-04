package workflow

import (
	"context"
	"fmt"
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
		return nil, err
	}

	return &PRResult{PR: pr}, nil
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
