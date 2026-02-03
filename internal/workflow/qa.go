package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/state"
)

// QAPhase handles the question-and-answer phase of issue processing
type QAPhase struct {
	claude   *claude.Client
	provider providers.Provider
}

// NewQAPhase creates a new QA phase handler
func NewQAPhase(claudeClient *claude.Client, provider providers.Provider) *QAPhase {
	return &QAPhase{
		claude:   claudeClient,
		provider: provider,
	}
}

// Result represents the result of a QA phase step
type QAResult struct {
	Questions       string
	NoMoreQuestions bool
	SessionID       string
}

// GenerateQuestions generates clarifying questions for an issue
func (q *QAPhase) GenerateQuestions(ctx context.Context, issue *providers.Issue, st *state.State, workDir string) (*QAResult, error) {
	qaHistory := claude.FormatQAHistory(st.QAHistory)

	prompt := fmt.Sprintf(claude.Prompts.AnalyzeIssue,
		issue.Title,
		issue.Body,
		qaHistory,
	)

	response, sessionID, err := q.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:   workDir,
		SessionID: st.SessionID,
		Prompt:    prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate questions: %w", err)
	}

	questions, noQuestionsNeeded := claude.ParseQuestionsResponse(response)

	return &QAResult{
		Questions:       questions,
		NoMoreQuestions: noQuestionsNeeded,
		SessionID:       sessionID,
	}, nil
}

// GenerateFollowUpQuestions generates follow-up questions based on previous answers
func (q *QAPhase) GenerateFollowUpQuestions(ctx context.Context, issue *providers.Issue, st *state.State, latestAnswers string, workDir string) (*QAResult, error) {
	qaHistory := claude.FormatQAHistory(st.QAHistory)

	prompt := fmt.Sprintf(claude.Prompts.FollowUpQuestions,
		issue.Title,
		qaHistory,
		latestAnswers,
	)

	response, sessionID, err := q.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:   workDir,
		SessionID: st.SessionID,
		Prompt:    prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate follow-up questions: %w", err)
	}

	questions, noQuestionsNeeded := claude.ParseQuestionsResponse(response)

	return &QAResult{
		Questions:       questions,
		NoMoreQuestions: noQuestionsNeeded,
		SessionID:       sessionID,
	}, nil
}

// PostQuestions posts questions as a comment on the issue
func (q *QAPhase) PostQuestions(ctx context.Context, repo string, issueNum int, questions string, roundNum int, st *state.State) error {
	commentBody := claude.FormatQuestionsForComment(questions, roundNum)

	// Append hidden state
	commentWithState, err := st.AppendToBody(commentBody)
	if err != nil {
		return fmt.Errorf("failed to append state: %w", err)
	}

	return q.provider.CreateComment(ctx, repo, issueNum, commentWithState)
}

// ParseUserAnswers extracts user answers from a comment
func ParseUserAnswers(comment string) string {
	// Remove any hidden state from the answer
	answer := state.RemoveState(comment)
	return strings.TrimSpace(answer)
}

// IsApproval checks if a comment is an approval
func IsApproval(comment string) bool {
	lower := strings.ToLower(strings.TrimSpace(comment))
	approvalPhrases := []string{
		"approved",
		"lgtm",
		"looks good to me",
		"looks good",
		"approve",
		"ship it",
		"go ahead",
		"proceed",
	}
	for _, phrase := range approvalPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// IsAbort checks if a comment is an abort command
func IsAbort(comment string) bool {
	lower := strings.ToLower(strings.TrimSpace(comment))
	return strings.HasPrefix(lower, "/abort") || lower == "abort"
}

// ExtractFeedback extracts feedback from a non-approval comment
func ExtractFeedback(comment string) string {
	// Remove hidden state
	feedback := state.RemoveState(comment)
	return strings.TrimSpace(feedback)
}
