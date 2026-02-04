package state

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/anthropics/ultra-engineer/internal/claude"
)

// Phase represents the current phase of issue processing
type Phase string

const (
	PhaseNew          Phase = "new"
	PhaseQuestions    Phase = "questions"
	PhasePlanning     Phase = "planning"
	PhaseApproval     Phase = "approval"
	PhaseImplementing Phase = "implementing"
	PhaseReview       Phase = "review"
	PhaseCompleted    Phase = "completed"
	PhaseFailed       Phase = "failed"
)

// PhaseLabel returns the label name for a phase
func (p Phase) Label() string {
	return "phase:" + string(p)
}

// State represents the hidden state stored in issue comments
type State struct {
	SessionID       string           `json:"session_id,omitempty"`
	CurrentPhase    Phase            `json:"current_phase"`
	QAHistory       []claude.QAEntry `json:"qa_history,omitempty"`
	QARound         int              `json:"qa_round,omitempty"`
	PlanVersion     int              `json:"plan_version,omitempty"`
	ReviewIteration int              `json:"review_iteration,omitempty"`
	PRNumber        int              `json:"pr_number,omitempty"`
	BranchName      string           `json:"branch_name,omitempty"`
	LastUpdated     time.Time        `json:"last_updated"`
	LastCommentID   int64            `json:"last_comment_id,omitempty"`
	Error           string           `json:"error,omitempty"`

	// Dependency tracking for concurrent issue processing
	DependsOn     []int  `json:"depends_on,omitempty"`     // Issue numbers this issue depends on
	BlockedBy     []int  `json:"blocked_by,omitempty"`     // Currently blocking issue numbers
	FailureReason string `json:"failure_reason,omitempty"` // "merge_conflict", "dependency_cycle", "dependency_failed", etc.
}

const (
	stateMarkerStart = "<!-- ultra-engineer-state"
	stateMarkerEnd   = "-->"
	BotMarker        = "<!-- ultra-engineer -->"
)

var stateRegex = regexp.MustCompile(`<!-- ultra-engineer-state\s*([\s\S]*?)\s*-->`)

// NewState creates a new state for an issue
func NewState() *State {
	return &State{
		CurrentPhase: PhaseNew,
		LastUpdated:  time.Now(),
	}
}

// Parse extracts state from a comment body
func Parse(body string) (*State, error) {
	matches := stateRegex.FindStringSubmatch(body)
	if matches == nil || len(matches) < 2 {
		return nil, fmt.Errorf("no state found in body")
	}

	jsonStr := strings.TrimSpace(matches[1])
	var state State
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		return nil, fmt.Errorf("failed to parse state JSON: %w", err)
	}

	return &state, nil
}

// ParseFromComments finds and parses state from a list of comments
// Returns the most recent state found
func ParseFromComments(comments []string) (*State, error) {
	var latestState *State

	for _, body := range comments {
		state, err := Parse(body)
		if err != nil {
			continue
		}
		if latestState == nil || state.LastUpdated.After(latestState.LastUpdated) {
			latestState = state
		}
	}

	if latestState == nil {
		return nil, fmt.Errorf("no state found in any comment")
	}

	return latestState, nil
}

// Serialize converts state to an HTML comment string
func (s *State) Serialize() (string, error) {
	s.LastUpdated = time.Now()

	jsonBytes, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to serialize state: %w", err)
	}

	return fmt.Sprintf("%s\n%s\n%s", stateMarkerStart, string(jsonBytes), stateMarkerEnd), nil
}

// AppendToBody appends serialized state to a comment body
func (s *State) AppendToBody(body string) (string, error) {
	stateStr, err := s.Serialize()
	if err != nil {
		return "", err
	}

	// Remove any existing state
	body = stateRegex.ReplaceAllString(body, "")
	body = strings.TrimSpace(body)

	return body + "\n\n" + stateStr, nil
}

// UpdateBody updates an existing body with new state
func (s *State) UpdateBody(body string) (string, error) {
	stateStr, err := s.Serialize()
	if err != nil {
		return "", err
	}

	// Replace existing state or append
	if stateRegex.MatchString(body) {
		return stateRegex.ReplaceAllString(body, stateStr), nil
	}

	return body + "\n\n" + stateStr, nil
}

// ContainsState checks if a body contains state
func ContainsState(body string) bool {
	return stateRegex.MatchString(body)
}

// IsBotComment checks if a comment was made by the bot (has bot marker or state)
func IsBotComment(body string) bool {
	return strings.Contains(body, BotMarker) || stateRegex.MatchString(body)
}

// AddBotMarker adds the bot marker to a comment body
func AddBotMarker(body string) string {
	return body + "\n\n" + BotMarker
}

// RemoveState removes state from a body
func RemoveState(body string) string {
	return strings.TrimSpace(stateRegex.ReplaceAllString(body, ""))
}

// SetPhase updates the phase and records the time
func (s *State) SetPhase(phase Phase) {
	s.CurrentPhase = phase
	s.LastUpdated = time.Now()
}

// AddQA adds a Q&A entry to the history
// Note: Does not increment QARound as that should be managed externally
// to avoid double-increment when this is called after incrementing in the orchestrator
func (s *State) AddQA(questions, answers string) {
	s.QAHistory = append(s.QAHistory, claude.QAEntry{
		Questions: questions,
		Answers:   answers,
	})
}

// IncrementReviewIteration increments and returns the review iteration
func (s *State) IncrementReviewIteration() int {
	s.ReviewIteration++
	return s.ReviewIteration
}

// ResetReviewIteration resets the review iteration counter
func (s *State) ResetReviewIteration() {
	s.ReviewIteration = 0
}

// Labels manages the labels for phase transitions
type Labels struct {
	allPhases []Phase
}

// NewLabels creates a Labels manager
func NewLabels() *Labels {
	return &Labels{
		allPhases: []Phase{
			PhaseQuestions,
			PhasePlanning,
			PhaseApproval,
			PhaseImplementing,
			PhaseReview,
			PhaseCompleted,
			PhaseFailed,
		},
	}
}

// GetPhaseLabelsToRemove returns labels to remove when transitioning to a new phase
func (l *Labels) GetPhaseLabelsToRemove(newPhase Phase) []string {
	var toRemove []string
	for _, p := range l.allPhases {
		if p != newPhase {
			toRemove = append(toRemove, p.Label())
		}
	}
	return toRemove
}

// ParsePhaseFromLabels extracts the current phase from labels
func ParsePhaseFromLabels(labels []string) Phase {
	for _, label := range labels {
		if strings.HasPrefix(label, "phase:") {
			phaseStr := strings.TrimPrefix(label, "phase:")
			return Phase(phaseStr)
		}
	}
	return PhaseNew
}
