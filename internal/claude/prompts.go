package claude

import (
	"fmt"
	"strings"
)

// Prompts contains all the prompt templates used by the orchestrator
var Prompts = struct {
	AnalyzeIssue string
	ReviewPlan   string
	ReviewCode   string
	Implement    string
	FixCI        string
}{
	AnalyzeIssue: `Analyze this issue and decide if you need clarifying questions.

Issue Title: %s
Issue Body:
%s

If you have clarifying questions, write them to .ultra-engineer/questions.md in this format:
1. [Question]
   A. [Option]
   B. [Option]
   C. Other (please specify)

If no questions needed, write "NO_QUESTIONS_NEEDED" to .ultra-engineer/questions.md

Then write your implementation plan to .ultra-engineer/plan.md with:
- Overview
- Files to create/modify
- Step-by-step approach
- Testing approach`,

	ReviewPlan: `Review the implementation plan at .ultra-engineer/plan.md (iteration %d/5).

Check for:
1. Missing edge cases
2. Security issues
3. Scope creep
4. Correct dependencies
5. Simpler alternatives
6. Testability

If issues found, fix them directly in .ultra-engineer/plan.md.
Output "REVIEW_COMPLETE" when done.`,

	ReviewCode: `Review the code changes (iteration %d/5).

Read .ultra-engineer/plan.md for context.

Check for:
1. Bugs and edge cases
2. Security vulnerabilities
3. Code readability
4. Performance issues
5. Missing error handling

Fix any issues directly in the code files.
Output "REVIEW_COMPLETE" when done.`,

	Implement: `Implement the plan from .ultra-engineer/plan.md

Issue: %s

Create/modify files as specified in the plan.
Output "IMPLEMENTATION_COMPLETE" when done.`,

	FixCI: `CI failed. Fix the issues.

Error:
%s

Fix the code and output "FIX_COMPLETE" when done.`,
}

// QAEntry represents a Q&A round
type QAEntry struct {
	Questions string
	Answers   string
}

// FormatQAHistory formats Q&A history for inclusion in prompts
func FormatQAHistory(qa []QAEntry) string {
	if len(qa) == 0 {
		return "(none)"
	}

	var sb strings.Builder
	for i, entry := range qa {
		sb.WriteString(fmt.Sprintf("Round %d:\n", i+1))
		sb.WriteString(fmt.Sprintf("Questions:\n%s\n", entry.Questions))
		sb.WriteString(fmt.Sprintf("Answers:\n%s\n\n", entry.Answers))
	}
	return sb.String()
}

// FormatQuestionsForComment formats questions for posting as an issue comment
func FormatQuestionsForComment(questions string, roundNum int) string {
	var sb strings.Builder
	sb.WriteString("## Questions\n\n")
	if roundNum > 1 {
		sb.WriteString(fmt.Sprintf("*Follow-up questions (round %d):*\n\n", roundNum))
	}
	sb.WriteString(questions)
	sb.WriteString("\n\n---\n")
	sb.WriteString("Reply with your answers (e.g., \"1A, 2B\" or write detailed responses).\n")
	return sb.String()
}

// FormatPlanForComment formats the plan for posting as an issue comment
func FormatPlanForComment(plan string, reviewCount int) string {
	var sb strings.Builder
	sb.WriteString("## Implementation Plan\n\n")
	sb.WriteString(fmt.Sprintf("*Reviewed %d times*\n\n", reviewCount))
	sb.WriteString(plan)
	sb.WriteString("\n\n---\n")
	sb.WriteString("Reply with:\n")
	sb.WriteString("- **approved** or **lgtm** to proceed with implementation\n")
	sb.WriteString("- Your feedback to request changes\n")
	return sb.String()
}
