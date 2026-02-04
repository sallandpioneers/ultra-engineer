package claude

import (
	"fmt"
	"strings"
)

// Prompts contains all the prompt templates used by the orchestrator
var Prompts = struct {
	AnalyzeIssue     string
	ReviewPlan       string
	ReviewCode       string
	Implement        string
	ImplementGit     string // Implementation with git commit/push to branch
	FixCI            string
	SummarizeChanges string
}{
	AnalyzeIssue: `Analyze this issue and decide if you need clarifying questions.

Issue Title: %s
Issue Body:
%s

If you have clarifying questions, write them to .ultra-engineer/questions.md in this format:

1. [Question]

   A. [Option] (Recommended)
      **Effort:** [Low/Medium/High]
      **Risk:** [Low/Medium/High - breaking changes, compatibility issues]
      **Pros:** [2-3 benefits]
      **Cons:** [1-2 drawbacks]

   B. [Option]
      **Effort:** [Low/Medium/High]
      **Risk:** [Low/Medium/High]
      **Pros:** [2-3 benefits]
      **Cons:** [1-2 drawbacks]

   C. Other (please specify)

Mark your recommended option with "(Recommended)". Add blank lines between options.
If an option depends on another question's answer, note it (e.g., "Requires 1A").

End with: "If you're unsure, replying with just the recommended options (e.g., '1A, 2A, 3B') is a safe default."

If no questions needed, write "NO_QUESTIONS_NEEDED" to .ultra-engineer/questions.md

Then write your implementation plan to .ultra-engineer/plan.md with:
- Overview
- Files to create/modify
- Step-by-step approach
- Testing approach`,

	ReviewPlan: `/review the plan at .ultra-engineer/plan.md and fix all issues`,

	ReviewCode: `/review the code and fix all issues`,

	Implement: `Implement the plan from .ultra-engineer/plan.md`,

	ImplementGit: `Implement the plan from .ultra-engineer/plan.md

Issue #%d: %s
Base branch: %s

After implementing the code changes:

## 1. Create a branch
Choose a descriptive branch name based on the issue (e.g., feat/add-user-auth, fix/login-timeout).
- git checkout -b <your-branch-name>

## 2. Commit your changes
Write meaningful commit messages:
- Use conventional commits: type(scope): description
  - feat: new feature
  - fix: bug fix
  - refactor: code change that neither fixes a bug nor adds a feature
  - docs: documentation only
  - test: adding or fixing tests
  - chore: maintenance tasks
- Explain WHY in the commit body, not just what
- Create multiple commits if changes are logically separate
- Reference the issue in your final commit: "Closes #%d"

Example:
  git add <files>
  git commit -m "feat(auth): add session timeout handling

  Users were getting logged out unexpectedly. Added configurable
  timeout with refresh token support.

  Closes #%d"

## 3. Integrate upstream changes
- git fetch origin %s
- Prefer rebase for clean history: git rebase origin/%s
- If rebase conflicts are too complex, use merge: git merge origin/%s
- Resolve conflicts using your understanding of the code
- If you cannot resolve a conflict, output:
  MERGE_CONFLICT_UNRESOLVED: <comma-separated list of files>

## 4. Push the branch
- git push -u origin <your-branch-name>
- If push fails due to remote changes, fetch/rebase and retry

Output "IMPLEMENTATION_COMPLETE <branch-name>" when done.`,

	FixCI: `CI failed. Fix the issues.

Error:
%s

Fix the code and output "FIX_COMPLETE" when done.`,

	SummarizeChanges: `Summarize the code changes for a PR description.

Run git diff origin/%s...%s to see the changes, then provide a concise summary in this format:

## Summary
[1-2 sentences describing what was implemented]

## Changes
[List each file changed with a brief description, e.g.:]
- ` + "`path/to/file.go`" + ` - Description of changes

## Notes
[Any notable decisions, trade-offs, or testing notes. Omit if none.]

Keep it brief and focus on the "what" and "why". Do not include markdown code blocks in your response.`,
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
	sb.WriteString("Reply `/approve` to proceed with implementation, or provide feedback to request changes.\n")
	return sb.String()
}
