package claude

import (
	"fmt"
	"strings"
)

// Prompts contains all the prompt templates used by the orchestrator
var Prompts = struct {
	AnalyzeIssue    string
	GenerateQuestions string
	FollowUpQuestions string
	CreatePlan      string
	ReviewPlan      string
	ReviewCode      string
	Implement       string
	FixCI           string
	AddressFeedback string
}{
	AnalyzeIssue: `You are analyzing an issue to understand what needs to be implemented.

Issue Title: %s
Issue Body:
%s

Previous Q&A (if any):
%s

Analyze this issue carefully. Your goal is to understand exactly what the user wants.

If you have clarifying questions, output them in this EXACT format:

QUESTIONS:
1. [Your first question]
   A. [First option]
   B. [Second option]
   C. [Third option]
   D. Other (please specify)

2. [Your second question]
   A. [First option]
   B. [Second option]
   C. Other (please specify)

Rules for questions:
- Number questions starting from 1
- Letter options starting from A
- ALWAYS include "Other (please specify)" as the last option
- Keep questions focused and specific
- Maximum 5 questions per batch
- If you have no questions and understand the requirements, output:
NO_QUESTIONS_NEEDED

Provide your questions or indicate no questions needed.`,

	GenerateQuestions: `Based on the issue and context, generate clarifying questions.

Issue: %s
Body: %s

Generate questions in this format:
QUESTIONS:
1. [Question]
   A. [Option]
   B. [Option]
   C. Other (please specify)

Or if no questions needed:
NO_QUESTIONS_NEEDED`,

	FollowUpQuestions: `Based on the user's answers, do you have follow-up questions?

Original Issue: %s

Previous Q&A:
%s

Latest Answers:
%s

If you have follow-up questions, output them in the same format:
QUESTIONS:
1. [Question]
   A. [Option]
   ...

If no more questions needed, output:
NO_QUESTIONS_NEEDED`,

	CreatePlan: `Create an implementation plan for this issue.

Issue Title: %s
Issue Body:
%s

Q&A Summary:
%s

Create a detailed implementation plan. Include:
1. Overview of what will be implemented
2. Files to be created or modified
3. Step-by-step implementation approach
4. Any dependencies or prerequisites
5. Testing approach

Format the plan in clear markdown.`,

	ReviewPlan: `Review iteration %d/5 for the implementation plan.

Analyze the plan critically:
1. Are there any missing edge cases or error scenarios?
2. Are there security vulnerabilities in the approach?
3. Is the scope creep-free (only what's needed)?
4. Are dependencies and order of operations correct?
5. Are there simpler alternatives to any complex parts?
6. Is it testable? How will we verify it works?
7. Does it match the user's requirements from the Q&A?

Current Plan:
%s

If you find issues, fix them directly and output the corrected plan.
If the plan is good, output it unchanged.

Be thorough - this is review %d of 5. Output the complete plan (corrected or unchanged).`,

	ReviewCode: `Review iteration %d/5 for the implementation.

Analyze the code critically:
1. Does it correctly implement the approved plan?
2. Are there bugs, edge cases, or error handling gaps?
3. Are there security vulnerabilities (injection, auth, etc.)?
4. Is the code readable and maintainable?
5. Are there any performance issues?
6. Does it follow the existing codebase patterns?
7. Are there missing tests for critical paths?

If you find issues, fix them directly in the code files.

Be thorough - this is review %d of 5. Fix any issues you find.`,

	Implement: `Implement the approved plan.

Issue Title: %s

Approved Plan:
%s

Implement this plan step by step. Create or modify files as needed.
Follow the existing code patterns in the repository.
Write clean, maintainable code with appropriate error handling.

After implementation, provide a summary of what was done.`,

	FixCI: `The CI pipeline failed. Fix the issues.

CI Error Output:
%s

Analyze the errors and fix them. Common issues include:
- Test failures
- Linting errors
- Build errors
- Type errors

Fix all issues and ensure the code passes CI.`,

	AddressFeedback: `Address the user's feedback on the implementation.

User Feedback:
%s

Current Implementation Summary:
%s

Address each point of feedback. Make the necessary changes.
Provide a summary of what was changed.`,
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

// QAEntry represents a Q&A round
type QAEntry struct {
	Questions string
	Answers   string
}

// ParseQuestionsResponse parses Claude's response to extract questions
func ParseQuestionsResponse(response string) (questions string, noQuestionsNeeded bool) {
	response = strings.TrimSpace(response)

	if strings.Contains(response, "NO_QUESTIONS_NEEDED") {
		return "", true
	}

	// Find the QUESTIONS: section
	if idx := strings.Index(response, "QUESTIONS:"); idx != -1 {
		questions = strings.TrimSpace(response[idx+len("QUESTIONS:"):])
		return questions, false
	}

	// If response looks like questions (numbered list), use it
	if strings.HasPrefix(response, "1.") || strings.HasPrefix(response, "1)") {
		return response, false
	}

	// No clear questions found
	return "", true
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
