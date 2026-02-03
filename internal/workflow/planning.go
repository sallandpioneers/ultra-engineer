package workflow

import (
	"context"
	"fmt"
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

// PlanResult represents the result of planning
type PlanResult struct {
	Plan      string
	SessionID string
}

// CreatePlan generates an implementation plan
func (p *PlanningPhase) CreatePlan(ctx context.Context, issue *providers.Issue, st *state.State, workDir string) (*PlanResult, error) {
	qaHistory := claude.FormatQAHistory(st.QAHistory)

	prompt := fmt.Sprintf(claude.Prompts.CreatePlan,
		issue.Title,
		issue.Body,
		qaHistory,
	)

	response, sessionID, err := p.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:   workDir,
		SessionID: st.SessionID,
		Prompt:    prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create plan: %w", err)
	}

	return &PlanResult{
		Plan:      response,
		SessionID: sessionID,
	}, nil
}

// ReviewPlan runs a single review iteration on the plan
func (p *PlanningPhase) ReviewPlan(ctx context.Context, plan string, iteration int, st *state.State, workDir string) (*PlanResult, error) {
	prompt := fmt.Sprintf(claude.Prompts.ReviewPlan,
		iteration,
		plan,
		iteration,
	)

	response, sessionID, err := p.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:   workDir,
		SessionID: st.SessionID,
		Prompt:    prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to review plan (iteration %d): %w", iteration, err)
	}

	return &PlanResult{
		Plan:      response,
		SessionID: sessionID,
	}, nil
}

// RunFullReviewCycle runs all review iterations on the plan
func (p *PlanningPhase) RunFullReviewCycle(ctx context.Context, initialPlan string, st *state.State, workDir string, progressCallback func(iteration int)) (*PlanResult, error) {
	currentPlan := initialPlan
	var lastSessionID string

	for i := 1; i <= p.reviewCycles; i++ {
		// Check for context cancellation before each iteration
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if progressCallback != nil {
			progressCallback(i)
		}

		result, err := p.ReviewPlan(ctx, currentPlan, i, st, workDir)
		if err != nil {
			return nil, err
		}

		currentPlan = result.Plan
		lastSessionID = result.SessionID
		st.SessionID = lastSessionID
	}

	return &PlanResult{
		Plan:      currentPlan,
		SessionID: lastSessionID,
	}, nil
}

// PostPlan posts the plan for user approval
func (p *PlanningPhase) PostPlan(ctx context.Context, repo string, issueNum int, plan string, st *state.State) error {
	commentBody := claude.FormatPlanForComment(plan, p.reviewCycles)

	// Append hidden state
	commentWithState, err := st.AppendToBody(commentBody)
	if err != nil {
		return fmt.Errorf("failed to append state: %w", err)
	}

	return p.provider.CreateComment(ctx, repo, issueNum, commentWithState)
}

// IntegrateFeedback integrates user feedback into the plan
func (p *PlanningPhase) IntegrateFeedback(ctx context.Context, plan string, feedback string, st *state.State, workDir string) (*PlanResult, bool, error) {
	prompt := fmt.Sprintf(`The user has provided feedback on the implementation plan.

Current Plan:
%s

User Feedback:
%s

Tasks:
1. Integrate the feedback into the plan
2. Determine if the changes are significant enough to warrant a full re-review

If changes are minor (typos, clarifications, small adjustments):
- Make the changes
- Output: MINOR_CHANGES
- Then output the updated plan

If changes are significant (new requirements, architectural changes, scope changes):
- Make the changes
- Output: SIGNIFICANT_CHANGES
- Then output the updated plan

Output the marker first, then the complete updated plan.`, plan, feedback)

	response, sessionID, err := p.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:   workDir,
		SessionID: st.SessionID,
		Prompt:    prompt,
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to integrate feedback: %w", err)
	}

	// Check if significant changes
	needsReReview := false
	updatedPlan := response

	if containsMarker(response, "SIGNIFICANT_CHANGES") {
		needsReReview = true
		updatedPlan = removeMarker(response, "SIGNIFICANT_CHANGES")
	} else if containsMarker(response, "MINOR_CHANGES") {
		updatedPlan = removeMarker(response, "MINOR_CHANGES")
	}

	// Validate that we have a non-empty plan
	updatedPlan = strings.TrimSpace(updatedPlan)
	if updatedPlan == "" {
		// If Claude only returned the marker without a plan, keep the original
		updatedPlan = plan
	}

	return &PlanResult{
		Plan:      updatedPlan,
		SessionID: sessionID,
	}, needsReReview, nil
}

func containsMarker(text, marker string) bool {
	if len(text) == 0 {
		return false
	}
	// Check if the marker appears anywhere in the text
	return strings.Contains(text, marker)
}

func removeMarker(text, marker string) string {
	// Find and remove the marker from the text
	idx := strings.Index(text, marker)
	if idx == -1 {
		return text
	}
	// Remove the marker and any leading whitespace before it
	before := strings.TrimRight(text[:idx], " \t\n")
	after := strings.TrimSpace(text[idx+len(marker):])
	if before == "" {
		return after
	}
	if after == "" {
		return before
	}
	return before + "\n\n" + after
}
