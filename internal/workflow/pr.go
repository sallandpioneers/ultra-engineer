package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/ultra-engineer/internal/providers"
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
func (p *PRPhase) CreatePR(ctx context.Context, repo string, issue *providers.Issue, headBranch, baseBranch string) (*PRResult, error) {
	prBody := p.formatPRBody(issue)

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

func (p *PRPhase) formatPRBody(issue *providers.Issue) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Summary\n\nImplements #%d\n\n", issue.Number))
	sb.WriteString("See the issue for the implementation plan and discussion.\n\n")
	sb.WriteString("---\n*Automated by Ultra Engineer*\n")
	return sb.String()
}
