package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/config"
	"github.com/anthropics/ultra-engineer/internal/progress"
	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/sandbox"
	"github.com/anthropics/ultra-engineer/internal/state"
	"github.com/anthropics/ultra-engineer/internal/workflow"
)

const (
	// NeedsManualResolutionLabel is added when merge conflicts cannot be resolved automatically
	NeedsManualResolutionLabel = "needs-manual-resolution"
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
		prPhase:   workflow.NewPRPhase(provider, claudeClient),
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
	// Create progress reporter for this issue
	reporter := progress.NewReporter(
		o.provider,
		repo,
		issue.Number,
		o.config.Progress.DebounceInterval,
		o.config.Progress.Enabled,
	)

	for {
		o.logger.Printf("Phase: %s", st.CurrentPhase)

		switch st.CurrentPhase {
		case state.PhaseNew:
			if err := o.handleNew(ctx, repo, issue, st, sb, reporter); err != nil {
				return o.fail(ctx, repo, issue.Number, st, err, reporter)
			}

		case state.PhaseQuestions:
			done, err := o.handleQuestions(ctx, repo, issue, st, sb, reporter)
			if err != nil {
				return o.fail(ctx, repo, issue.Number, st, err, reporter)
			}
			if done {
				return nil // Waiting for user
			}

		case state.PhasePlanning:
			if err := o.handlePlanning(ctx, repo, issue, st, sb, reporter); err != nil {
				return o.fail(ctx, repo, issue.Number, st, err, reporter)
			}

		case state.PhaseApproval:
			done, err := o.handleApproval(ctx, repo, issue, st, sb, reporter)
			if err != nil {
				return o.fail(ctx, repo, issue.Number, st, err, reporter)
			}
			if done {
				return nil // Waiting for user
			}

		case state.PhaseImplementing:
			if err := o.handleImplementing(ctx, repo, issue, st, sb, reporter); err != nil {
				return o.fail(ctx, repo, issue.Number, st, err, reporter)
			}

		case state.PhaseReview:
			done, err := o.handleReview(ctx, repo, issue, st, sb, reporter)
			if err != nil {
				return o.fail(ctx, repo, issue.Number, st, err, reporter)
			}
			if done {
				return nil
			}

		case state.PhaseCompleted:
			o.logger.Printf("Issue #%d completed", issue.Number)
			reporter.Finalize(ctx, progress.FormatCompleted(st.PRNumber))
			return nil

		case state.PhaseFailed:
			return fmt.Errorf("issue failed: %s", st.Error)
		}
	}
}

func (o *Orchestrator) handleNew(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox, reporter *progress.Reporter) error {
	o.logger.Printf("Analyzing issue...")
	reporter.ForceUpdate(ctx, progress.StatusAnalyzing)

	result, err := o.qaPhase.AnalyzeIssue(ctx, issue, sb.RepoDir)
	if err != nil {
		return err
	}

	if result.NoMoreQuestions {
		st.SetPhase(state.PhasePlanning)
		o.setLabel(ctx, repo, issue.Number, state.PhasePlanning)
	} else {
		st.QARound = 1
		st.SetPhase(state.PhaseQuestions) // Set phase BEFORE posting so state is correct
		if err := o.qaPhase.PostQuestions(ctx, repo, issue.Number, result.Questions, 1, st); err != nil {
			return err
		}
		o.setLabel(ctx, repo, issue.Number, state.PhaseQuestions)
	}
	return nil
}

func (o *Orchestrator) handleQuestions(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox, reporter *progress.Reporter) (bool, error) {
	comments, err := o.provider.GetComments(ctx, repo, issue.Number)
	if err != nil {
		return false, err
	}

	// Find latest user answer (skip bot comments)
	var answer *providers.Comment
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]
		if c.ID > st.LastCommentID && !state.IsBotComment(c.Body) {
			answer = c
			break
		}
	}

	if answer == nil {
		return true, nil // Wait for user
	}

	// React to acknowledge we've read the comment
	o.provider.ReactToComment(ctx, repo, answer.ID, "+1")

	if workflow.IsAbort(answer.Body) {
		return false, fmt.Errorf("user aborted")
	}

	st.LastCommentID = answer.ID
	// Move to planning (simplified - skip follow-up questions for now)
	st.SetPhase(state.PhasePlanning)
	o.setLabel(ctx, repo, issue.Number, state.PhasePlanning)
	return false, nil
}

func (o *Orchestrator) handlePlanning(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox, reporter *progress.Reporter) error {
	o.logger.Printf("Running %d plan reviews...", o.config.Claude.ReviewCycles)
	reporter.ForceUpdate(ctx, progress.StatusPlanning)

	totalCycles := o.config.Claude.ReviewCycles
	err := o.planPhase.RunFullReviewCycle(ctx, sb.RepoDir, func(i int) {
		o.logger.Printf("Plan review %d/%d", i, totalCycles)
		reporter.Update(ctx, progress.FormatPlanReview(i, totalCycles))
	})
	if err != nil {
		return err
	}

	plan, err := o.planPhase.GetPlan(sb.RepoDir)
	if err != nil {
		return fmt.Errorf("failed to read plan: %w", err)
	}

	st.SetPhase(state.PhaseApproval)

	if err := o.planPhase.PostPlan(ctx, repo, issue.Number, plan, st); err != nil {
		return err
	}

	reporter.ForceUpdate(ctx, progress.StatusWaitingApproval)
	o.setLabel(ctx, repo, issue.Number, state.PhaseApproval)
	return nil
}

func (o *Orchestrator) handleApproval(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox, reporter *progress.Reporter) (bool, error) {
	comments, err := o.provider.GetComments(ctx, repo, issue.Number)
	if err != nil {
		return false, err
	}

	// Find latest user response (skip bot comments)
	var response *providers.Comment
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]
		if c.ID > st.LastCommentID && !state.IsBotComment(c.Body) {
			response = c
			break
		}
	}

	if response == nil {
		return true, nil // Wait for user
	}

	// React to acknowledge we've read the comment
	o.provider.ReactToComment(ctx, repo, response.ID, "+1")

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
	reporter.ForceUpdate(ctx, progress.StatusPlanning)

	needsReReview, err := o.planPhase.IntegrateFeedback(ctx, feedback, sb.RepoDir)
	if err != nil {
		return false, err
	}

	if needsReReview {
		o.logger.Printf("Re-reviewing plan...")
		totalCycles := o.config.Claude.ReviewCycles
		o.planPhase.RunFullReviewCycle(ctx, sb.RepoDir, func(i int) {
			o.logger.Printf("Plan re-review %d/%d", i, totalCycles)
			reporter.Update(ctx, progress.FormatPlanReview(i, totalCycles))
		})
	}

	plan, _ := o.planPhase.GetPlan(sb.RepoDir)
	st.PlanVersion++

	if err := o.planPhase.PostPlan(ctx, repo, issue.Number, plan, st); err != nil {
		return false, err
	}

	reporter.ForceUpdate(ctx, progress.StatusWaitingApproval)
	return true, nil // Wait for approval again
}

func (o *Orchestrator) handleImplementing(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox, reporter *progress.Reporter) error {
	baseBranch := o.config.Defaults.BaseBranch
	if b, _ := o.provider.GetDefaultBranch(ctx, repo); b != "" {
		baseBranch = b
	}

	o.logger.Printf("Implementing with git operations...")
	reporter.ForceUpdate(ctx, progress.StatusImplementing)
	result, err := o.implPhase.ImplementWithGit(ctx, issue.Title, issue.Number, baseBranch, sb)
	if err != nil {
		return err
	}

	// Handle merge conflict
	if result.MergeConflict {
		return o.failWithMergeConflict(ctx, repo, issue.Number, st, result.ConflictingFiles, reporter)
	}

	// Store branch name from Claude's choice (for PR workflow)
	if result.BranchName != "" {
		st.BranchName = result.BranchName
	}

	o.logger.Printf("Running %d code reviews...", o.config.Claude.ReviewCycles)
	totalCycles := o.config.Claude.ReviewCycles
	err = o.implPhase.RunFullCodeReviewCycle(ctx, sb, func(i int) {
		o.logger.Printf("Code review %d/%d", i, totalCycles)
		reporter.Update(ctx, progress.FormatCodeReview(i, totalCycles))
	})
	if err != nil {
		return err
	}

	st.SetPhase(state.PhaseReview)
	o.setLabel(ctx, repo, issue.Number, state.PhaseReview)
	return nil
}

func (o *Orchestrator) handleReview(ctx context.Context, repo string, issue *providers.Issue, st *state.State, sb *sandbox.Sandbox, reporter *progress.Reporter) (bool, error) {
	if st.PRNumber == 0 {
		o.logger.Printf("Creating PR...")
		reporter.ForceUpdate(ctx, progress.StatusCreatingPR)
		baseBranch, _ := o.provider.GetDefaultBranch(ctx, repo)
		if baseBranch == "" {
			baseBranch = o.config.Defaults.BaseBranch
		}

		// Note: Claude already committed and pushed the branch during implementation
		// We just need to create the PR now

		pr, err := o.prPhase.CreatePR(ctx, repo, issue, st.BranchName, baseBranch, sb.RepoDir)
		if err != nil {
			return false, err
		}

		st.PRNumber = pr.PR.Number
		o.logger.Printf("Created PR #%d", st.PRNumber)

		// Initialize LastPRCommentTime after PR creation to avoid processing old comments
		st.LastPRCommentTime = time.Now()

		o.provider.CreateComment(ctx, repo, issue.Number, state.AddBotMarker(fmt.Sprintf("Created PR #%d: %s", st.PRNumber, pr.PR.HTMLURL)))
	}

	// Check for PR feedback (general comments and inline review comments)
	var allComments []*providers.Comment

	prComments, err := o.provider.GetPRComments(ctx, repo, st.PRNumber)
	if err != nil {
		o.logger.Printf("Warning: failed to fetch PR comments: %v", err)
	} else {
		allComments = append(allComments, prComments...)
	}

	reviewComments, err := o.provider.GetPRReviewComments(ctx, repo, st.PRNumber)
	if err != nil {
		o.logger.Printf("Warning: failed to fetch PR review comments: %v", err)
	} else {
		allComments = append(allComments, reviewComments...)
	}

	// Filter for new comments using CreatedAt timestamp (not ID)
	// This handles the fact that general comments and review comments have different ID spaces
	var newFeedback []string
	var latestTime time.Time
	for _, c := range allComments {
		if c.CreatedAt.After(st.LastPRCommentTime) && !state.IsBotComment(c.Body) {
			newFeedback = append(newFeedback, c.Body)
			if c.CreatedAt.After(latestTime) {
				latestTime = c.CreatedAt
			}
		}
	}

	if len(newFeedback) > 0 {
		o.logger.Printf("Processing %d PR feedback comment(s)...", len(newFeedback))

		// Combine all feedback into one prompt
		combinedFeedback := strings.Join(newFeedback, "\n\n---\n\n")

		// Address the feedback - Claude fixes code AND handles git operations
		if err := o.implPhase.AddressFeedback(ctx, combinedFeedback, sb, st.BranchName); err != nil {
			return false, err
		}

		// Update state BEFORE posting acknowledgment
		st.LastPRCommentTime = latestTime

		// Post acknowledgment on the issue with updated state
		ackMsg := state.AddBotMarker("Addressed PR feedback and pushed changes.")
		msgWithState, _ := st.AppendToBody(ackMsg)
		o.provider.CreateComment(ctx, repo, issue.Number, msgWithState)

		// Return immediately to wait for CI to run on the new commit
		return true, nil
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

func (o *Orchestrator) fail(ctx context.Context, repo string, issueNum int, st *state.State, err error, reporter *progress.Reporter) error {
	o.logger.Printf("Error: %v", err)
	st.Error = err.Error()
	st.SetPhase(state.PhaseFailed)

	reporter.Finalize(ctx, progress.FormatFailed(err))

	comment := fmt.Sprintf("**Error:**\n```\n%s\n```", err.Error())
	commentWithState, _ := st.AppendToBody(comment)
	o.provider.CreateComment(ctx, repo, issueNum, commentWithState)
	o.setLabel(ctx, repo, issueNum, state.PhaseFailed)

	return err
}

// failWithMergeConflict handles the case when Claude cannot resolve a merge conflict
func (o *Orchestrator) failWithMergeConflict(ctx context.Context, repo string, issueNum int, st *state.State, conflictingFiles []string, reporter *progress.Reporter) error {
	o.logger.Printf("Merge conflict in files: %v", conflictingFiles)

	st.FailureReason = "merge_conflict"
	st.Error = fmt.Sprintf("Merge conflict in: %s", strings.Join(conflictingFiles, ", "))
	st.SetPhase(state.PhaseFailed)

	reporter.Finalize(ctx, progress.FormatFailed(fmt.Errorf("merge conflict")))

	// Format the conflict message
	var sb strings.Builder
	sb.WriteString("**Merge Conflict**\n\n")
	sb.WriteString("Unable to automatically resolve merge conflicts in the following files:\n\n")
	for _, f := range conflictingFiles {
		sb.WriteString(fmt.Sprintf("- `%s`\n", f))
	}
	sb.WriteString("\n**What was attempted:**\n")
	sb.WriteString("- Fetched latest changes from origin/main\n")
	sb.WriteString("- Attempted to rebase onto main\n")
	sb.WriteString("- Tried to resolve conflicts using code context\n\n")
	sb.WriteString("**To resolve:**\n")
	sb.WriteString("1. Manually resolve the conflicts in the listed files\n")
	sb.WriteString("2. Push the resolved changes to the branch\n")
	sb.WriteString("3. Comment `/retry` to re-trigger processing\n")

	commentWithState, _ := st.AppendToBody(sb.String())
	o.provider.CreateComment(ctx, repo, issueNum, commentWithState)

	// Update labels
	o.setLabel(ctx, repo, issueNum, state.PhaseFailed)
	o.provider.RemoveLabel(ctx, repo, issueNum, o.config.TriggerLabel)
	o.provider.AddLabel(ctx, repo, issueNum, NeedsManualResolutionLabel)

	return fmt.Errorf("merge conflict: %s", strings.Join(conflictingFiles, ", "))
}

// CheckForRetry checks if a failed issue has a /retry comment and should be retried
func (o *Orchestrator) CheckForRetry(ctx context.Context, repo string, issue *providers.Issue, st *state.State) bool {
	// Only check issues that need manual resolution
	hasNeedsManualLabel := false
	for _, label := range issue.Labels {
		if label == NeedsManualResolutionLabel {
			hasNeedsManualLabel = true
			break
		}
	}
	if !hasNeedsManualLabel {
		return false
	}

	// Check for /retry comment after the failure
	comments, err := o.provider.GetComments(ctx, repo, issue.Number)
	if err != nil {
		return false
	}

	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]
		if c.ID > st.LastCommentID && !state.IsBotComment(c.Body) {
			body := strings.TrimSpace(strings.ToLower(c.Body))
			if body == "/retry" || strings.HasPrefix(body, "/retry ") {
				// Found retry command - reset state for retry
				o.logger.Printf("Retry requested for issue #%d", issue.Number)

				st.FailureReason = ""
				st.Error = ""
				st.SetPhase(state.PhaseImplementing)
				st.LastCommentID = c.ID

				// Update labels
				o.provider.RemoveLabel(ctx, repo, issue.Number, NeedsManualResolutionLabel)
				o.provider.RemoveLabel(ctx, repo, issue.Number, state.PhaseFailed.Label())
				o.provider.AddLabel(ctx, repo, issue.Number, o.config.TriggerLabel)
				o.setLabel(ctx, repo, issue.Number, state.PhaseImplementing)

				// React to acknowledge
				o.provider.ReactToComment(ctx, repo, c.ID, "+1")

				// Post comment about retry
				o.provider.CreateComment(ctx, repo, issue.Number,
					state.AddBotMarker("Retrying implementation..."))

				return true
			}
		}
	}

	return false
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

// CanProceed checks if all dependencies are satisfied (completed, not just in-progress)
// repoStates is the subset of allStates for the given repo (issueNum -> state)
func (o *Orchestrator) CanProceed(ctx context.Context, repo string, issue *providers.Issue, st *state.State, repoStates map[int]*state.State) bool {
	if len(st.DependsOn) == 0 {
		return true
	}

	for _, depNum := range st.DependsOn {
		depState, exists := repoStates[depNum]
		if !exists {
			// Dependency not tracked - might be completed or doesn't exist
			// Try to check via provider
			depIssue, err := o.provider.GetIssue(ctx, repo, depNum)
			if err != nil {
				// Can't find dependency - block to be safe
				return false
			}
			depPhase := state.ParsePhaseFromLabels(depIssue.Labels)
			if depPhase != state.PhaseCompleted {
				return false
			}
			continue
		}

		// Check if dependency is completed
		if depState.CurrentPhase != state.PhaseCompleted {
			return false
		}
	}

	return true
}

// CheckAndUnblockIssues re-evaluates blocked issues after any issue completes or fails
// If a dependency failed, dependent issues should also be marked as failed
// Returns list of newly-ready issues that should be submitted to the worker pool
func (o *Orchestrator) CheckAndUnblockIssues(ctx context.Context, repo string, completedOrFailedIssue int, repoStates map[int]*state.State) ([]*providers.Issue, error) {
	var readyIssues []*providers.Issue

	completedState, exists := repoStates[completedOrFailedIssue]
	if !exists {
		return nil, nil
	}

	// Check each issue that might depend on the completed/failed issue
	for issueNum, st := range repoStates {
		if issueNum == completedOrFailedIssue {
			continue
		}

		// Skip if not blocked or already processing
		if len(st.BlockedBy) == 0 {
			continue
		}

		// Check if this issue was blocked by the completed/failed one
		wasBlocked := false
		for _, blockedBy := range st.BlockedBy {
			if blockedBy == completedOrFailedIssue {
				wasBlocked = true
				break
			}
		}

		if !wasBlocked {
			continue
		}

		// If dependency failed, mark this issue as failed too
		if completedState.CurrentPhase == state.PhaseFailed {
			st.CurrentPhase = state.PhaseFailed
			st.FailureReason = "dependency_failed"
			st.Error = fmt.Sprintf("Dependency #%d failed", completedOrFailedIssue)

			// Post comment about the failure
			comment := fmt.Sprintf("**Blocked:** Dependency #%d failed. This issue cannot proceed until the dependency is resolved.\n\nRetry with `/retry` after fixing the dependency.", completedOrFailedIssue)
			commentWithState, _ := st.AppendToBody(comment)
			o.provider.CreateComment(ctx, repo, issueNum, commentWithState)
			o.setLabel(ctx, repo, issueNum, state.PhaseFailed)
			continue
		}

		// Dependency completed - update blocked_by
		var newBlockedBy []int
		for _, b := range st.BlockedBy {
			if b != completedOrFailedIssue {
				newBlockedBy = append(newBlockedBy, b)
			}
		}
		st.BlockedBy = newBlockedBy

		// If no longer blocked, add to ready list
		if len(st.BlockedBy) == 0 {
			// Get the issue from provider
			issue, err := o.provider.GetIssue(ctx, repo, issueNum)
			if err != nil {
				o.logger.Printf("Failed to get issue #%d: %v", issueNum, err)
				continue
			}

			// Post comment that we're unblocked
			comment := fmt.Sprintf("Dependency #%d completed. Proceeding with this issue.", completedOrFailedIssue)
			o.provider.CreateComment(ctx, repo, issueNum, state.AddBotMarker(comment))

			readyIssues = append(readyIssues, issue)
		}
	}

	return readyIssues, nil
}
