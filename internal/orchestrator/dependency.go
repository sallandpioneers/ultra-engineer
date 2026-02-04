package orchestrator

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/providers"
)

// DependencyDetector detects dependencies between issues
type DependencyDetector struct {
	provider providers.Provider
	claude   *claude.Client
	mode     string // "auto", "manual", or "disabled"
}

// NewDependencyDetector creates a new dependency detector
func NewDependencyDetector(provider providers.Provider, claudeClient *claude.Client, mode string) *DependencyDetector {
	if mode == "" {
		mode = "auto"
	}
	return &DependencyDetector{
		provider: provider,
		claude:   claudeClient,
		mode:     mode,
	}
}

// DetectDependencies detects dependencies for an issue based on the configured mode
func (d *DependencyDetector) DetectDependencies(ctx context.Context, repo string, issue *providers.Issue) ([]int, error) {
	if d.mode == "disabled" {
		return nil, nil
	}

	// Check for explicit override labels or comments
	if d.hasNoDepsOverride(issue) {
		return nil, nil
	}

	// Parse explicit references from issue content
	deps := d.ParseIssueReferences(issue.Body)

	// Also check comments for dependency declarations
	if comments, err := d.provider.GetComments(ctx, repo, issue.Number); err == nil {
		for _, comment := range comments {
			commentDeps := d.ParseIssueReferences(comment.Body)
			deps = append(deps, commentDeps...)
		}
	}

	// Remove duplicates and self-references
	deps = d.deduplicateDeps(deps, issue.Number)

	return deps, nil
}

// ParseIssueReferences finds issue references in text
// Supports: #123, "depends on #456", "after #789", "requires #123", "blocked by #456"
func (d *DependencyDetector) ParseIssueReferences(text string) []int {
	var deps []int

	// Pattern for explicit dependency declarations
	dependencyPatterns := []string{
		`(?i)depends?\s+on\s+#(\d+)`,
		`(?i)after\s+#(\d+)`,
		`(?i)requires?\s+#(\d+)`,
		`(?i)blocked\s+by\s+#(\d+)`,
		`(?i)waiting\s+(?:for|on)\s+#(\d+)`,
	}

	for _, pattern := range dependencyPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) > 1 {
				if num, err := strconv.Atoi(match[1]); err == nil {
					deps = append(deps, num)
				}
			}
		}
	}

	return deps
}

// CheckForCycles detects cycles in the dependency graph
// issues is a map of issueNumber -> list of issue numbers it depends on
func (d *DependencyDetector) CheckForCycles(issues map[int][]int) error {
	// Track visited and currently in recursion stack
	visited := make(map[int]bool)
	recStack := make(map[int]bool)
	var cyclePath []int

	var dfs func(node int) bool
	dfs = func(node int) bool {
		visited[node] = true
		recStack[node] = true
		cyclePath = append(cyclePath, node)

		for _, dep := range issues[node] {
			if !visited[dep] {
				if dfs(dep) {
					return true
				}
			} else if recStack[dep] {
				// Found cycle - find where it starts
				cyclePath = append(cyclePath, dep)
				return true
			}
		}

		recStack[node] = false
		cyclePath = cyclePath[:len(cyclePath)-1]
		return false
	}

	for node := range issues {
		if !visited[node] {
			cyclePath = nil
			if dfs(node) {
				// Extract just the cycle portion
				cycleStart := -1
				for i, n := range cyclePath {
					if n == cyclePath[len(cyclePath)-1] && i < len(cyclePath)-1 {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := cyclePath[cycleStart:]
					return fmt.Errorf("dependency cycle detected: %s", formatCycle(cycle))
				}
				return fmt.Errorf("dependency cycle detected")
			}
		}
	}

	return nil
}

// hasNoDepsOverride checks if the issue has an override to skip dependency detection
func (d *DependencyDetector) hasNoDepsOverride(issue *providers.Issue) bool {
	// Check labels
	for _, label := range issue.Labels {
		if label == "no-dependencies" {
			return true
		}
	}

	// Check for /no-deps comment (this would need to be checked via comments)
	return strings.Contains(issue.Body, "/no-deps")
}

// deduplicateDeps removes duplicates and self-references from deps
func (d *DependencyDetector) deduplicateDeps(deps []int, selfIssueNum int) []int {
	seen := make(map[int]bool)
	var result []int

	for _, dep := range deps {
		if dep != selfIssueNum && !seen[dep] {
			seen[dep] = true
			result = append(result, dep)
		}
	}

	return result
}

// formatCycle formats a cycle path for display
func formatCycle(cycle []int) string {
	parts := make([]string, len(cycle))
	for i, n := range cycle {
		parts[i] = fmt.Sprintf("#%d", n)
	}
	return strings.Join(parts, " -> ")
}
