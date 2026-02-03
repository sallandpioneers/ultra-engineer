package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/sandbox"
)

// PRPhase handles the PR creation and merge phase
type PRPhase struct {
	provider providers.Provider
}

// NewPRPhase creates a new PR phase handler
func NewPRPhase(provider providers.Provider) *PRPhase {
	return &PRPhase{provider: provider}
}

// PRResult represents the result of PR operations
type PRResult struct {
	PR     *providers.PR
	Merged bool
}

// CreatePR creates a pull request from the implementation
func (p *PRPhase) CreatePR(ctx context.Context, repo string, issue *providers.Issue, plan string, sb *sandbox.Sandbox, baseBranch string) (*PRResult, error) {
	prBody := p.formatPRBody(issue, plan)

	pr, err := p.provider.CreatePR(ctx, repo, providers.PRCreate{
		Title:   fmt.Sprintf("Implement: %s", issue.Title),
		Body:    prBody,
		Head:    sb.BranchName,
		Base:    baseBranch,
		IssueID: issue.Number,
	})
	if err != nil {
		return nil, err
	}

	return &PRResult{PR: pr}, nil
}

func (p *PRPhase) formatPRBody(issue *providers.Issue, plan string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Summary\n\nImplements #%d\n\n", issue.Number))
	sb.WriteString("## Plan\n\n<details>\n<summary>Click to expand</summary>\n\n")
	sb.WriteString(plan)
	sb.WriteString("\n\n</details>\n\n")
	sb.WriteString("---\n*Automated by Ultra Engineer*\n")
	return sb.String()
}
