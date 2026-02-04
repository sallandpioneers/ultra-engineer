package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/state"
)

// PlanningPhase handles the planning phase of issue processing
type PlanningPhase struct {
	claude       *claude.Client
	provider     providers.Provider
	reviewCycles int
}

// NewPlanningPhase creates a new planning phase handler
func NewPlanningPhase(claudeClient *claude.Client, provider providers.Provider, reviewCycles int) *PlanningPhase {
	return &PlanningPhase{
		claude:       claudeClient,
		provider:     provider,
		reviewCycles: reviewCycles,
	}
}

// ReviewPlan runs a single review iteration on the plan
func (p *PlanningPhase) ReviewPlan(ctx context.Context, iteration int, workDir string) error {
	prompt := fmt.Sprintf(claude.Prompts.ReviewPlan, iteration)

	_, _, err := p.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      workDir,
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Write", "Edit"},
	})
	return err
}

// RunFullReviewCycle runs all review iterations on the plan
func (p *PlanningPhase) RunFullReviewCycle(ctx context.Context, workDir string, progressCallback func(iteration int)) error {
	for i := 1; i <= p.reviewCycles; i++ {
		if progressCallback != nil {
			progressCallback(i)
		}
		if err := p.ReviewPlan(ctx, i, workDir); err != nil {
			return err
		}
	}
	return nil
}

// GetPlan reads the plan from the file
func (p *PlanningPhase) GetPlan(workDir string) (string, error) {
	planPath := filepath.Join(workDir, ".ultra-engineer", "plan.md")
	data, err := os.ReadFile(planPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// PostPlan posts the plan for user approval
func (p *PlanningPhase) PostPlan(ctx context.Context, repo string, issueNum int, plan string, st *state.State) error {
	commentBody := claude.FormatPlanForComment(plan, p.reviewCycles)
	commentWithState, err := st.AppendToBody(commentBody)
	if err != nil {
		return err
	}
	_, err = p.provider.CreateComment(ctx, repo, issueNum, commentWithState)
	return err
}

// IntegrateFeedback writes feedback to a file for Claude to process
func (p *PlanningPhase) IntegrateFeedback(ctx context.Context, feedback string, workDir string) (bool, error) {
	// Write feedback to file
	feedbackPath := filepath.Join(workDir, ".ultra-engineer", "feedback.md")
	os.WriteFile(feedbackPath, []byte(feedback), 0644)

	prompt := `Read the user feedback at .ultra-engineer/feedback.md. This feedback is a CHANGE REQUEST - the user wants you to modify the plan, not explain or justify the current approach.

Revise .ultra-engineer/plan.md to incorporate the user's requested changes:
- If they ask "can X do Y?" or "why not X?" - they're requesting you change the approach to use X
- If they disagree with a decision - change the plan to use their preferred approach
- If they suggest additions - add them to the plan
- Do NOT add explanatory sections defending the current approach

After updating the plan, output:
- "SIGNIFICANT_CHANGES" if the changes affect architecture, approach, or requirements
- "MINOR_CHANGES" if the changes are clarifications or small additions`

	result, _, err := p.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      workDir,
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Write", "Edit"},
	})
	if err != nil {
		return false, err
	}

	return strings.Contains(result, "SIGNIFICANT_CHANGES"), nil
}
