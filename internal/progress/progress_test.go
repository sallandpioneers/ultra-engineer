package progress

import (
	"context"
	"testing"
	"time"

	"github.com/anthropics/ultra-engineer/internal/providers"
)

func TestReporter_FirstUpdateCreatesComment(t *testing.T) {
	mock := providers.NewMockProvider()
	mock.AddIssue("owner/repo", &providers.Issue{Number: 1})

	reporter := NewReporter(mock, "owner/repo", 1, 60*time.Second, true)

	err := reporter.ForceUpdate(context.Background(), StatusAnalyzing)
	if err != nil {
		t.Fatalf("ForceUpdate failed: %v", err)
	}

	if len(mock.CreatedComments) != 1 {
		t.Fatalf("Expected 1 comment created, got %d", len(mock.CreatedComments))
	}

	if mock.CreatedComments[0].IssueNum != 1 {
		t.Errorf("Expected issue 1, got %d", mock.CreatedComments[0].IssueNum)
	}
}

func TestReporter_SubsequentUpdateEditsComment(t *testing.T) {
	mock := providers.NewMockProvider()
	mock.AddIssue("owner/repo", &providers.Issue{Number: 1})

	reporter := NewReporter(mock, "owner/repo", 1, 60*time.Second, true)

	// First update creates comment
	reporter.ForceUpdate(context.Background(), StatusAnalyzing)

	// Second update should edit the existing comment
	err := reporter.ForceUpdate(context.Background(), StatusPlanning)
	if err != nil {
		t.Fatalf("ForceUpdate failed: %v", err)
	}

	if len(mock.CreatedComments) != 1 {
		t.Errorf("Expected only 1 comment created, got %d", len(mock.CreatedComments))
	}

	if len(mock.UpdatedComments) != 1 {
		t.Fatalf("Expected 1 comment update, got %d", len(mock.UpdatedComments))
	}
}

func TestReporter_UpdateRespectsDebounce(t *testing.T) {
	mock := providers.NewMockProvider()
	mock.AddIssue("owner/repo", &providers.Issue{Number: 1})

	// Use a long debounce interval
	reporter := NewReporter(mock, "owner/repo", 1, 10*time.Second, true)

	// First update creates comment
	reporter.ForceUpdate(context.Background(), StatusAnalyzing)

	// Immediate Update should be debounced (skipped)
	err := reporter.Update(context.Background(), StatusPlanning)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Should have no updates because of debounce
	if len(mock.UpdatedComments) != 0 {
		t.Errorf("Expected 0 updates (debounced), got %d", len(mock.UpdatedComments))
	}
}

func TestReporter_ForceUpdateBypassesDebounce(t *testing.T) {
	mock := providers.NewMockProvider()
	mock.AddIssue("owner/repo", &providers.Issue{Number: 1})

	// Use a long debounce interval
	reporter := NewReporter(mock, "owner/repo", 1, 10*time.Second, true)

	// First update creates comment
	reporter.ForceUpdate(context.Background(), StatusAnalyzing)

	// ForceUpdate should bypass debounce
	err := reporter.ForceUpdate(context.Background(), StatusPlanning)
	if err != nil {
		t.Fatalf("ForceUpdate failed: %v", err)
	}

	// Should have 1 update despite short interval
	if len(mock.UpdatedComments) != 1 {
		t.Errorf("Expected 1 update (force bypasses debounce), got %d", len(mock.UpdatedComments))
	}
}

func TestReporter_FinalizeAlwaysPosts(t *testing.T) {
	mock := providers.NewMockProvider()
	mock.AddIssue("owner/repo", &providers.Issue{Number: 1})

	reporter := NewReporter(mock, "owner/repo", 1, 10*time.Second, true)

	// First update creates comment
	reporter.ForceUpdate(context.Background(), StatusAnalyzing)

	// Finalize should always post
	err := reporter.Finalize(context.Background(), FormatCompleted(123))
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	if len(mock.UpdatedComments) != 1 {
		t.Errorf("Expected 1 update from Finalize, got %d", len(mock.UpdatedComments))
	}
}

func TestReporter_DisabledDoesNothing(t *testing.T) {
	mock := providers.NewMockProvider()
	mock.AddIssue("owner/repo", &providers.Issue{Number: 1})

	reporter := NewReporter(mock, "owner/repo", 1, 60*time.Second, false) // disabled

	err := reporter.ForceUpdate(context.Background(), StatusAnalyzing)
	if err != nil {
		t.Fatalf("ForceUpdate failed: %v", err)
	}

	if len(mock.CreatedComments) != 0 {
		t.Errorf("Expected no comments when disabled, got %d", len(mock.CreatedComments))
	}
}

func TestFormatStatusMessages(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() string
		contains string
	}{
		{"PlanReview", func() string { return FormatPlanReview(2, 5) }, "2/5"},
		{"CodeReview", func() string { return FormatCodeReview(3, 5) }, "3/5"},
		{"CompletedWithPR", func() string { return FormatCompleted(123) }, "PR #123"},
		{"CompletedNoPR", func() string { return FormatCompleted(0) }, "Completed successfully"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn()
			if !containsStr(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s[1:], substr) || s[:len(substr)] == substr)
}
