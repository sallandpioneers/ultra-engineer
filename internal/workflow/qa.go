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

// QAResult represents the result of a QA phase step
type QAResult struct {
	Questions       string
	Plan            string
	NoMoreQuestions bool
}

// AnalyzeIssue analyzes the issue and generates questions + initial plan
func (q *QAPhase) AnalyzeIssue(ctx context.Context, issue *providers.Issue, workDir string) (*QAResult, error) {
	// Create .ultra-engineer directory
	ueDir := filepath.Join(workDir, ".ultra-engineer")
	os.MkdirAll(ueDir, 0755)

	prompt := fmt.Sprintf(claude.Prompts.AnalyzeIssue, issue.Title, issue.Body)

	_, _, err := q.claude.RunInteractive(ctx, claude.RunOptions{
		WorkDir:      workDir,
		Prompt:       prompt,
		AllowedTools: []string{"Read", "Write", "Glob", "Grep"},
	})
	if err != nil {
		return nil, err
	}

	// Read questions file
	questionsPath := filepath.Join(ueDir, "questions.md")
	questionsData, _ := os.ReadFile(questionsPath)
	questions := strings.TrimSpace(string(questionsData))

	// Read plan file
	planPath := filepath.Join(ueDir, "plan.md")
	planData, _ := os.ReadFile(planPath)
	plan := strings.TrimSpace(string(planData))

	noQuestions := strings.Contains(questions, "NO_QUESTIONS_NEEDED") || questions == ""

	return &QAResult{
		Questions:       questions,
		Plan:            plan,
		NoMoreQuestions: noQuestions,
	}, nil
}

// PostQuestions posts questions as a comment on the issue
func (q *QAPhase) PostQuestions(ctx context.Context, repo string, issueNum int, questions string, roundNum int, st *state.State) error {
	commentBody := claude.FormatQuestionsForComment(questions, roundNum)
	commentWithState, err := st.AppendToBody(commentBody)
	if err != nil {
		return err
	}
	return q.provider.CreateComment(ctx, repo, issueNum, commentWithState)
}

// ParseUserAnswers extracts user answers from a comment
func ParseUserAnswers(comment string) string {
	answer := state.RemoveState(comment)
	return strings.TrimSpace(answer)
}

// IsApproval checks if a comment is an approval (only /approve)
func IsApproval(comment string) bool {
	trimmed := strings.TrimSpace(comment)
	return trimmed == "/approve"
}

// IsAbort checks if a comment is an abort command
func IsAbort(comment string) bool {
	lower := strings.ToLower(strings.TrimSpace(comment))
	return strings.HasPrefix(lower, "/abort") || lower == "abort"
}

// ExtractFeedback extracts feedback from a non-approval comment
func ExtractFeedback(comment string) string {
	return strings.TrimSpace(state.RemoveState(comment))
}
