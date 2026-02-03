package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/config"
	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/sandbox"
	"github.com/anthropics/ultra-engineer/internal/state"
	"github.com/anthropics/ultra-engineer/internal/workflow"
)

// Orchestrator coordinates the issue processing workflow
type Orchestrator struct {
	config   *config.Config
	provider providers.Provider
	claude   *claude.Client
	sandbox  *sandbox.Manager
	logger   *log.Logger

	qaPhase   *workflow.QAPhase
	planPhase *workflow.PlanningPhase
	implPhase *workflow.ImplementationPhase
	prPhase   *workflow.PRPhase
}

// New creates a new orchestrator
func New(cfg *config.Config, provider providers.Provider, logger *log.Logger) *Orchestrator {
	claudeClient := claude.NewClient(cfg.Claude.Command, cfg.Claude.Timeout)
	sandboxMgr := sandbox.NewManager("")

	return &Orchestrator{
		config:    cfg,
		provider:  provider,
		claude:    claudeClient,
		sandbox:   sandboxMgr,
		logger:    logger,
		qaPhase:   workflow.NewQAPhase(claudeClient, provider),
		planPhase: workflow.NewPlanningPhase(claudeClient, provider, cfg.Claude.ReviewCycles),
		implPhase: workflow.NewImplementationPhase(claudeClient, provider, cfg.Claude.ReviewCycles),
		prPhase:   workflow.NewPRPhase(provider),
	}
}

// ProcessIssue processes a single issue through the workflow
func (o *Orchestrator) ProcessIssue(ctx context.Context, repo string, issue *providers.Issue) error {
	o.logger.Printf("Processing issue #%d: %s", issue.Number, issue.Title)

	// Get or create sandbox
	issueID := fmt.Sprintf("%s-%d", repo, issue.Number)
	sb, err := o.sandbox.GetOrCreate(repo, issueID)
	if err != nil {
		return fmt.Errorf("failed to create sandbox: %w", err)
	}

	// Load or create state
	st, err := o.loadState(ctx, repo, issue.Number)
	if err != nil {
		st = state.NewState()
		phase := state.ParsePhaseFromLabels(issue.Labels)
		if phase != state.PhaseNew {
			st.CurrentPhase = phase
		}
	}

	// Clone repo if needed
	if !sb.Exists() {
		o.logger.Printf("Cloning repository...")
		if err := o.provider.Clone(ctx, repo, sb.RepoDir); err != nil {
			return fmt.Errorf("failed to clone: %w", err)
		}
	}

	return o.runStateMachine(ctx, repo, issue, st, sb)
}

func (o *Orchestrator) loadState(ctx context.Context, repo string, issueNum int) (*state.State, error) {
	comments, err := o.provider.GetComments(ctx, repo, issueNum)
	if err != nil {
		return nil, err
	}
	var bodies []string
	for _, c := range comments {
		bodies = append(bodies, c.Body)
	}
	return state.ParseFromComments(bodies)
}

func (o *Orchestrator) runStateMachine(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) error {
	for {
		o.logger.Printf("Phase: %s", st.CurrentPhase)

		switch st.CurrentPhase {
		case state.PhaseNew:
			if err := o.handleNew(ctx, repo, issue, st, sb); err != nil {
				return o.fail(ctx, repo, issue.Number, st, err)
			}

		case state.PhaseQuestions:
			done, err := o.handleQuestions(ctx, repo, issue, st, sb)
			if err != nil {
				return o.fail(ctx, repo, issue.Number, st, err)
			}
			if done {
				return nil // Waiting for user
			}

		case state.PhasePlanning:
			if err := o.handlePlanning(ctx, repo, issue, st, sb); err != nil {
				return o.fail(ctx, repo, issue.Number, st, err)
			}

		case state.PhaseApproval:
			done, err := o.handleApproval(ctx, repo, issue, st, sb)
			if err != nil {
				return o.fail(ctx, repo, issue.Number, st, err)
			}
			if done {
				return nil // Waiting for user
			}

		case state.PhaseImplementing:
			if err := o.handleImplementing(ctx, repo, issue, st, sb); err != nil {
				return o.fail(ctx, repo, issue.Number, st, err)
			}

		case state.PhaseReview:
			done, err := o.handleReview(ctx, repo, issue, st, sb)
			if err != nil {
				return o.fail(ctx, repo, issue.Number, st, err)
			}
			if done {
				return nil
			}

		case state.PhaseCompleted:
			o.logger.Printf("Issue #%d completed", issue.Number)
			return nil

		case state.PhaseFailed:
			return fmt.Errorf("issue failed: %s", st.Error)
		}
	}
}

func (o *Orchestrator) handleNew(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) error {
	o.logger.Printf("Analyzing issue...")

	result, err := o.qaPhase.AnalyzeIssue(ctx, issue, sb.RepoDir)
	if err != nil {
		return err
	}

	if result.NoMoreQuestions {
		st.SetPhase(state.PhasePlanning)
		o.setLabel(ctx, repo, issue.Number, state.PhasePlanning)
	} else {
		st.QARound = 1
		if err := o.qaPhase.PostQuestions(ctx, repo, issue.Number, result.Questions, 1, st); err != nil {
			return err
		}
		st.SetPhase(state.PhaseQuestions)
		o.setLabel(ctx, repo, issue.Number, state.PhaseQuestions)
	}
	return nil
}

func (o *Orchestrator) handleQuestions(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) (bool, error) {
	comments, err := o.provider.GetComments(ctx, repo, issue.Number)
	if err != nil {
		return false, err
	}

	// Find latest user answer
	var answer *providers.Comment
	for i := len(comments) - 1; i >= 0; i-- {
		if !state.ContainsState(comments[i].Body) && comments[i].ID > st.LastCommentID {
			answer = comments[i]
			break
		}
	}

	if answer == nil {
		return true, nil // Wait for user
	}

	if workflow.IsAbort(answer.Body) {
		return false, fmt.Errorf("user aborted")
	}

	st.LastCommentID = answer.ID
	// Move to planning (simplified - skip follow-up questions for now)
	st.SetPhase(state.PhasePlanning)
	o.setLabel(ctx, repo, issue.Number, state.PhasePlanning)
	return false, nil
}

func (o *Orchestrator) handlePlanning(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) error {
	o.logger.Printf("Running %d plan reviews...", o.config.Claude.ReviewCycles)

	err := o.planPhase.RunFullReviewCycle(ctx, sb.RepoDir, func(i int) {
		o.logger.Printf("Plan review %d/%d", i, o.config.Claude.ReviewCycles)
	})
	if err != nil {
		return err
	}

	plan, err := o.planPhase.GetPlan(sb.RepoDir)
	if err != nil {
		return fmt.Errorf("failed to read plan: %w", err)
	}

	st.Plan = plan
	st.SetPhase(state.PhaseApproval)

	if err := o.planPhase.PostPlan(ctx, repo, issue.Number, plan, st); err != nil {
		return err
	}

	o.setLabel(ctx, repo, issue.Number, state.PhaseApproval)
	return nil
}

func (o *Orchestrator) handleApproval(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) (bool, error) {
	comments, err := o.provider.GetComments(ctx, repo, issue.Number)
	if err != nil {
		return false, err
	}

	var response *providers.Comment
	for i := len(comments) - 1; i >= 0; i-- {
		if !state.ContainsState(comments[i].Body) && comments[i].ID > st.LastCommentID {
			response = comments[i]
			break
		}
	}

	if response == nil {
		return true, nil // Wait for user
	}

	st.LastCommentID = response.ID

	if workflow.IsAbort(response.Body) {
		return false, fmt.Errorf("user aborted")
	}

	if workflow.IsApproval(response.Body) {
		st.SetPhase(state.PhaseImplementing)
		o.setLabel(ctx, repo, issue.Number, state.PhaseImplementing)
		return false, nil
	}

	// Handle feedback
	feedback := workflow.ExtractFeedback(response.Body)
	o.logger.Printf("Integrating feedback...")

	needsReReview, err := o.planPhase.IntegrateFeedback(ctx, feedback, sb.RepoDir)
	if err != nil {
		return false, err
	}

	if needsReReview {
		o.logger.Printf("Re-reviewing plan...")
		o.planPhase.RunFullReviewCycle(ctx, sb.RepoDir, func(i int) {
			o.logger.Printf("Plan re-review %d/%d", i, o.config.Claude.ReviewCycles)
		})
	}

	plan, _ := o.planPhase.GetPlan(sb.RepoDir)
	st.Plan = plan
	st.PlanVersion++

	if err := o.planPhase.PostPlan(ctx, repo, issue.Number, plan, st); err != nil {
		return false, err
	}

	return true, nil // Wait for approval again
}

func (o *Orchestrator) handleImplementing(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) error {
	branchName := fmt.Sprintf("ultra-engineer/issue-%d", issue.Number)
	if err := sb.CreateBranch(ctx, branchName); err != nil {
		return err
	}
	st.BranchName = branchName

	o.logger.Printf("Implementing...")
	if err := o.implPhase.Implement(ctx, issue.Title, sb); err != nil {
		return err
	}

	o.logger.Printf("Running %d code reviews...", o.config.Claude.ReviewCycles)
	err := o.implPhase.RunFullCodeReviewCycle(ctx, sb, func(i int) {
		o.logger.Printf("Code review %d/%d", i, o.config.Claude.ReviewCycles)
	})
	if err != nil {
		return err
	}

	st.SetPhase(state.PhaseReview)
	o.setLabel(ctx, repo, issue.Number, state.PhaseReview)
	return nil
}

func (o *Orchestrator) handleReview(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) (bool, error) {
	if st.PRNumber == 0 {
		o.logger.Printf("Creating PR...")
		baseBranch, _ := o.provider.GetDefaultBranch(ctx, repo)
		if baseBranch == "" {
			baseBranch = o.config.Defaults.BaseBranch
		}

		// Commit and push
		if err := sb.Commit(ctx, fmt.Sprintf("Implement: %s\n\nCloses #%d", issue.Title, issue.Number)); err != nil {
			return false, err
		}
		if err := sb.Push(ctx); err != nil {
			return false, err
		}

		pr, err := o.prPhase.CreatePR(ctx, repo, issue, st.Plan, sb, baseBranch)
		if err != nil {
			return false, err
		}

		st.PRNumber = pr.PR.Number
		o.logger.Printf("Created PR #%d", st.PRNumber)

		o.provider.CreateComment(ctx, repo, issue.Number, fmt.Sprintf("Created PR #%d: %s", st.PRNumber, pr.PR.HTMLURL))
	}

	// Check if mergeable
	mergeable, err := o.provider.IsMergeable(ctx, repo, st.PRNumber)
	if err != nil {
		return false, err
	}

	if mergeable && o.config.Defaults.AutoMerge {
		o.logger.Printf("Merging PR #%d", st.PRNumber)
		if err := o.provider.MergePR(ctx, repo, st.PRNumber); err != nil {
			return false, err
		}
		st.SetPhase(state.PhaseCompleted)
		o.setLabel(ctx, repo, issue.Number, state.PhaseCompleted)
		sb.Cleanup()
		return false, nil
	}

	return true, nil // Wait for CI/reviews
}

func (o *Orchestrator) fail(ctx context.Context, repo string, issueNum int, st *state.State, err error) error {
	o.logger.Printf("Error: %v", err)
	st.Error = err.Error()
	st.SetPhase(state.PhaseFailed)

	comment := fmt.Sprintf("**Error:**\n```\n%s\n```", err.Error())
	commentWithState, _ := st.AppendToBody(comment)
	o.provider.CreateComment(ctx, repo, issueNum, commentWithState)
	o.setLabel(ctx, repo, issueNum, state.PhaseFailed)

	return err
}

func (o *Orchestrator) setLabel(ctx context.Context, repo string, issueNum int, phase state.Phase) {
	labels := state.NewLabels()
	for _, l := range labels.GetPhaseLabelsToRemove(phase) {
		o.provider.RemoveLabel(ctx, repo, issueNum, l)
	}
	o.provider.AddLabel(ctx, repo, issueNum, phase.Label())
}

func (o *Orchestrator) WaitForInteraction(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}
