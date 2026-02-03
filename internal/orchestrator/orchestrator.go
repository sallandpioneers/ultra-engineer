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

	// Workflow phases
	qaPhase     *workflow.QAPhase
	planPhase   *workflow.PlanningPhase
	implPhase   *workflow.ImplementationPhase
	prPhase     *workflow.PRPhase
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
		o.logger.Printf("No existing state found, starting fresh: %v", err)
		st = state.NewState()
	}

	// Clone repo if needed
	if !sb.Exists() {
		o.logger.Printf("Cloning repository...")
		if err := o.provider.Clone(ctx, repo, sb.RepoDir); err != nil {
			return fmt.Errorf("failed to clone repository: %w", err)
		}
	}

	// Run the state machine
	return o.runStateMachine(ctx, repo, issue, st, sb)
}

// loadState loads state from issue comments
func (o *Orchestrator) loadState(ctx context.Context, repo string, issueNum int) (*state.State, error) {
	comments, err := o.provider.GetComments(ctx, repo, issueNum)
	if err != nil {
		return nil, err
	}

	var commentBodies []string
	for _, c := range comments {
		commentBodies = append(commentBodies, c.Body)
	}

	return state.ParseFromComments(commentBodies)
}

// runStateMachine executes the workflow state machine
func (o *Orchestrator) runStateMachine(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		o.logger.Printf("Current phase: %s", st.CurrentPhase)

		var err error
		var done bool

		switch st.CurrentPhase {
		case state.PhaseNew:
			err = o.handleNewPhase(ctx, repo, issue, st, sb)
		case state.PhaseQuestions:
			done, err = o.handleQuestionsPhase(ctx, repo, issue, st, sb)
		case state.PhasePlanning:
			err = o.handlePlanningPhase(ctx, repo, issue, st, sb)
		case state.PhaseApproval:
			done, err = o.handleApprovalPhase(ctx, repo, issue, st, sb)
		case state.PhaseImplementing:
			err = o.handleImplementingPhase(ctx, repo, issue, st, sb)
		case state.PhaseReview:
			done, err = o.handleReviewPhase(ctx, repo, issue, st, sb)
		case state.PhaseCompleted:
			o.logger.Printf("Issue #%d completed successfully", issue.Number)
			return nil
		case state.PhaseFailed:
			o.logger.Printf("Issue #%d failed: %s", issue.Number, st.Error)
			return fmt.Errorf("issue processing failed: %s", st.Error)
		default:
			return fmt.Errorf("unknown phase: %s", st.CurrentPhase)
		}

		if err != nil {
			o.handleError(ctx, repo, issue.Number, st, err)
			return err
		}

		// If phase requires waiting for user input, exit the loop
		if done {
			return nil
		}
	}
}

// handleNewPhase handles a new issue
func (o *Orchestrator) handleNewPhase(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) error {
	o.logger.Printf("Starting Q&A phase for issue #%d", issue.Number)

	// Generate initial questions
	result, err := o.qaPhase.GenerateQuestions(ctx, issue, st, sb.RepoDir)
	if err != nil {
		return fmt.Errorf("failed to generate questions: %w", err)
	}

	st.SessionID = result.SessionID

	if result.NoMoreQuestions {
		// No questions needed, go straight to planning
		st.SetPhase(state.PhasePlanning)
		o.updateLabels(ctx, repo, issue.Number, state.PhasePlanning)
		return nil
	}

	// Post questions and wait for response
	st.QARound = 1
	if err := o.qaPhase.PostQuestions(ctx, repo, issue.Number, result.Questions, st.QARound, st); err != nil {
		return fmt.Errorf("failed to post questions: %w", err)
	}

	st.SetPhase(state.PhaseQuestions)
	o.updateLabels(ctx, repo, issue.Number, state.PhaseQuestions)
	return nil
}

// handleQuestionsPhase handles the Q&A phase
func (o *Orchestrator) handleQuestionsPhase(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) (bool, error) {
	// Check for new comments (answers)
	comments, err := o.provider.GetComments(ctx, repo, issue.Number)
	if err != nil {
		return false, fmt.Errorf("failed to get comments: %w", err)
	}

	// Find latest user answer
	var latestAnswer *providers.Comment
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]
		if !state.ContainsState(c.Body) && c.ID > st.LastCommentID {
			latestAnswer = c
			break
		}
	}

	if latestAnswer == nil {
		// No new answers, wait
		return true, nil
	}

	// Check for abort
	if workflow.IsAbort(latestAnswer.Body) {
		return false, o.abort(ctx, repo, issue.Number, st, "User requested abort")
	}

	// Process the answer
	answer := workflow.ParseUserAnswers(latestAnswer.Body)
	st.LastCommentID = latestAnswer.ID

	// Generate follow-up questions
	result, err := o.qaPhase.GenerateFollowUpQuestions(ctx, issue, st, answer, sb.RepoDir)
	if err != nil {
		return false, fmt.Errorf("failed to generate follow-up questions: %w", err)
	}

	st.SessionID = result.SessionID

	// Record Q&A
	if st.QARound > 0 {
		// Get the questions from the last round
		var lastQuestions string
		for i := len(comments) - 1; i >= 0; i-- {
			if state.ContainsState(comments[i].Body) {
				lastQuestions = state.RemoveState(comments[i].Body)
				break
			}
		}
		st.AddQA(lastQuestions, answer)
	}

	if result.NoMoreQuestions {
		// Move to planning phase
		st.SetPhase(state.PhasePlanning)
		o.updateLabels(ctx, repo, issue.Number, state.PhasePlanning)
		return false, nil
	}

	// Post follow-up questions
	st.QARound++
	if err := o.qaPhase.PostQuestions(ctx, repo, issue.Number, result.Questions, st.QARound, st); err != nil {
		return false, fmt.Errorf("failed to post follow-up questions: %w", err)
	}

	return true, nil
}

// handlePlanningPhase handles the planning phase
func (o *Orchestrator) handlePlanningPhase(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) error {
	o.logger.Printf("Creating implementation plan...")

	// Create initial plan
	planResult, err := o.planPhase.CreatePlan(ctx, issue, st, sb.RepoDir)
	if err != nil {
		return fmt.Errorf("failed to create plan: %w", err)
	}

	st.SessionID = planResult.SessionID
	st.Plan = planResult.Plan

	// Run review cycles
	o.logger.Printf("Running %d plan review cycles...", o.config.Claude.ReviewCycles)
	reviewedPlan, err := o.planPhase.RunFullReviewCycle(ctx, st.Plan, st, sb.RepoDir, func(iteration int) {
		o.logger.Printf("Plan review iteration %d/%d", iteration, o.config.Claude.ReviewCycles)
	})
	if err != nil {
		return fmt.Errorf("failed to review plan: %w", err)
	}

	st.Plan = reviewedPlan.Plan
	st.SessionID = reviewedPlan.SessionID
	st.PlanVersion++

	// Post plan for approval
	if err := o.planPhase.PostPlan(ctx, repo, issue.Number, st.Plan, st); err != nil {
		return fmt.Errorf("failed to post plan: %w", err)
	}

	st.SetPhase(state.PhaseApproval)
	o.updateLabels(ctx, repo, issue.Number, state.PhaseApproval)
	return nil
}

// handleApprovalPhase handles waiting for plan approval
func (o *Orchestrator) handleApprovalPhase(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) (bool, error) {
	// Check for new comments
	comments, err := o.provider.GetComments(ctx, repo, issue.Number)
	if err != nil {
		return false, fmt.Errorf("failed to get comments: %w", err)
	}

	// Find latest user response
	var latestResponse *providers.Comment
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]
		if !state.ContainsState(c.Body) && c.ID > st.LastCommentID {
			latestResponse = c
			break
		}
	}

	if latestResponse == nil {
		// No new response, wait
		return true, nil
	}

	st.LastCommentID = latestResponse.ID

	// Check for abort
	if workflow.IsAbort(latestResponse.Body) {
		return false, o.abort(ctx, repo, issue.Number, st, "User requested abort")
	}

	// Check for approval
	if workflow.IsApproval(latestResponse.Body) {
		st.SetPhase(state.PhaseImplementing)
		o.updateLabels(ctx, repo, issue.Number, state.PhaseImplementing)
		return false, nil
	}

	// Handle feedback
	feedback := workflow.ExtractFeedback(latestResponse.Body)
	o.logger.Printf("Received feedback, integrating...")

	result, needsReReview, err := o.planPhase.IntegrateFeedback(ctx, st.Plan, feedback, st, sb.RepoDir)
	if err != nil {
		return false, fmt.Errorf("failed to integrate feedback: %w", err)
	}

	st.Plan = result.Plan
	st.SessionID = result.SessionID

	if needsReReview {
		o.logger.Printf("Significant changes, running re-review cycle...")
		reviewedPlan, err := o.planPhase.RunFullReviewCycle(ctx, st.Plan, st, sb.RepoDir, func(iteration int) {
			o.logger.Printf("Plan re-review iteration %d/%d", iteration, o.config.Claude.ReviewCycles)
		})
		if err != nil {
			return false, fmt.Errorf("failed to re-review plan: %w", err)
		}
		st.Plan = reviewedPlan.Plan
		st.SessionID = reviewedPlan.SessionID
	}

	st.PlanVersion++

	// Post updated plan
	if err := o.planPhase.PostPlan(ctx, repo, issue.Number, st.Plan, st); err != nil {
		return false, fmt.Errorf("failed to post updated plan: %w", err)
	}

	return true, nil
}

// handleImplementingPhase handles the implementation phase
func (o *Orchestrator) handleImplementingPhase(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) error {
	o.logger.Printf("Implementing approved plan...")

	// Create feature branch
	branchName := fmt.Sprintf("ultra-engineer/issue-%d", issue.Number)
	if err := sb.CreateBranch(ctx, branchName); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	st.BranchName = branchName

	// Implement
	implResult, err := o.implPhase.Implement(ctx, issue, st.Plan, st, sb)
	if err != nil {
		return fmt.Errorf("failed to implement: %w", err)
	}

	st.SessionID = implResult.SessionID

	// Run code review cycles
	o.logger.Printf("Running %d code review cycles...", o.config.Claude.ReviewCycles)
	_, err = o.implPhase.RunFullCodeReviewCycle(ctx, st, sb, func(iteration int) {
		o.logger.Printf("Code review iteration %d/%d", iteration, o.config.Claude.ReviewCycles)
	})
	if err != nil {
		return fmt.Errorf("failed to review code: %w", err)
	}

	// Move to PR phase
	st.SetPhase(state.PhaseReview)
	o.updateLabels(ctx, repo, issue.Number, state.PhaseReview)
	return nil
}

// handleReviewPhase handles the PR and merge phase
func (o *Orchestrator) handleReviewPhase(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox) (bool, error) {
	// Create PR if not exists
	if st.PRNumber == 0 {
		o.logger.Printf("Creating pull request...")

		baseBranch, err := o.provider.GetDefaultBranch(ctx, repo)
		if err != nil {
			baseBranch = o.config.Defaults.BaseBranch
		}

		prResult, err := o.prPhase.CreatePR(ctx, repo, issue, st.Plan, sb, baseBranch)
		if err != nil {
			return false, fmt.Errorf("failed to create PR: %w", err)
		}

		st.PRNumber = prResult.PR.Number
		o.logger.Printf("Created PR #%d: %s", st.PRNumber, prResult.PR.HTMLURL)

		// Post link to issue
		comment := fmt.Sprintf("Created PR #%d for this issue.\n\n%s", st.PRNumber, prResult.PR.HTMLURL)
		if err := o.provider.CreateComment(ctx, repo, issue.Number, comment); err != nil {
			o.logger.Printf("Warning: failed to post PR link: %v", err)
		}
	}

	// Check PR status
	checkResult, err := o.prPhase.CheckPRStatus(ctx, repo, st.PRNumber, st.LastCommentID)
	if err != nil {
		return false, fmt.Errorf("failed to check PR status: %w", err)
	}

	// Handle abort
	if checkResult.HasFeedback && checkResult.Feedback == "/abort" {
		return false, o.abort(ctx, repo, issue.Number, st, "User requested abort")
	}

	// Handle user feedback
	if checkResult.HasFeedback {
		o.logger.Printf("Addressing PR feedback...")
		_, err := o.implPhase.AddressFeedback(ctx, checkResult.Feedback, "", st, sb)
		if err != nil {
			return false, fmt.Errorf("failed to address feedback: %w", err)
		}

		// Push the fix
		if err := o.prPhase.PushFix(ctx, sb, "Address review feedback"); err != nil {
			return false, fmt.Errorf("failed to push fix: %w", err)
		}

		return true, nil
	}

	// Handle CI failure
	if checkResult.CIFailed {
		o.logger.Printf("Fixing CI failure...")
		_, err := o.implPhase.FixCIFailure(ctx, checkResult.CIOutput, st, sb)
		if err != nil {
			return false, fmt.Errorf("failed to fix CI: %w", err)
		}

		// Push the fix
		if err := o.prPhase.PushFix(ctx, sb, "Fix CI failure"); err != nil {
			return false, fmt.Errorf("failed to push CI fix: %w", err)
		}

		return true, nil
	}

	// Check if mergeable
	if checkResult.IsMergeable && o.config.Defaults.AutoMerge {
		o.logger.Printf("Merging PR #%d...", st.PRNumber)
		if err := o.prPhase.Merge(ctx, repo, st.PRNumber); err != nil {
			return false, fmt.Errorf("failed to merge PR: %w", err)
		}

		st.SetPhase(state.PhaseCompleted)
		o.updateLabels(ctx, repo, issue.Number, state.PhaseCompleted)

		// Cleanup sandbox
		if err := sb.Cleanup(); err != nil {
			o.logger.Printf("Warning: failed to cleanup sandbox: %v", err)
		}

		return false, nil
	}

	// Wait for CI / reviews
	return true, nil
}

// handleError handles an error during processing
func (o *Orchestrator) handleError(ctx context.Context, repo string, issueNum int, st *state.State, err error) {
	o.logger.Printf("Error processing issue: %v", err)

	st.Error = err.Error()
	st.SetPhase(state.PhaseFailed)

	// Post error comment
	comment := fmt.Sprintf("**Error during processing:**\n\n```\n%s\n```\n\nPlease check the logs for more details.", err.Error())
	commentWithState, _ := st.AppendToBody(comment)
	o.provider.CreateComment(ctx, repo, issueNum, commentWithState)

	o.updateLabels(ctx, repo, issueNum, state.PhaseFailed)
}

// abort aborts processing of an issue
func (o *Orchestrator) abort(ctx context.Context, repo string, issueNum int, st *state.State, reason string) error {
	o.logger.Printf("Aborting issue #%d: %s", issueNum, reason)

	st.Error = reason
	st.SetPhase(state.PhaseFailed)

	// Post abort comment
	comment := fmt.Sprintf("**Processing aborted:** %s", reason)
	commentWithState, _ := st.AppendToBody(comment)
	o.provider.CreateComment(ctx, repo, issueNum, commentWithState)

	o.updateLabels(ctx, repo, issueNum, state.PhaseFailed)

	// Remove trigger label
	o.provider.RemoveLabel(ctx, repo, issueNum, o.config.TriggerLabel)

	return fmt.Errorf("aborted: %s", reason)
}

// updateLabels updates phase labels on an issue
func (o *Orchestrator) updateLabels(ctx context.Context, repo string, issueNum int, newPhase state.Phase) {
	labels := state.NewLabels()

	// Remove old phase labels
	for _, label := range labels.GetPhaseLabelsToRemove(newPhase) {
		o.provider.RemoveLabel(ctx, repo, issueNum, label)
	}

	// Add new phase label
	o.provider.AddLabel(ctx, repo, issueNum, newPhase.Label())
}

// WaitForInteraction waits for the specified duration before checking again
func (o *Orchestrator) WaitForInteraction(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}
