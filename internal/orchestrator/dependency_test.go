package orchestrator

import (
	"testing"
)

func TestParseIssueReferences(t *testing.T) {
	detector := NewDependencyDetector(nil, nil, "auto")

	tests := []struct {
		name     string
		input    string
		expected []int
	}{
		{
			name:     "depends on #123",
			input:    "This issue depends on #123",
			expected: []int{123},
		},
		{
			name:     "depend on #456",
			input:    "depend on #456",
			expected: []int{456},
		},
		{
			name:     "after #789",
			input:    "This should be done after #789",
			expected: []int{789},
		},
		{
			name:     "requires #100",
			input:    "requires #100 to be completed first",
			expected: []int{100},
		},
		{
			name:     "require #101",
			input:    "require #101",
			expected: []int{101},
		},
		{
			name:     "blocked by #200",
			input:    "Currently blocked by #200",
			expected: []int{200},
		},
		{
			name:     "waiting for #300",
			input:    "waiting for #300",
			expected: []int{300},
		},
		{
			name:     "waiting on #301",
			input:    "waiting on #301",
			expected: []int{301},
		},
		{
			name:     "multiple dependencies",
			input:    "depends on #1, also requires #2 and blocked by #3",
			expected: []int{1, 2, 3},
		},
		{
			name:     "no dependencies",
			input:    "This is a standalone issue",
			expected: nil,
		},
		{
			name:     "case insensitive",
			input:    "DEPENDS ON #999",
			expected: []int{999},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.ParseIssueReferences(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d dependencies, got %d", len(tt.expected), len(result))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("expected %d at index %d, got %d", tt.expected[i], i, v)
				}
			}
		})
	}
}

func TestCheckForCycles(t *testing.T) {
	detector := NewDependencyDetector(nil, nil, "auto")

	tests := []struct {
		name        string
		issues      map[int][]int
		expectError bool
	}{
		{
			name: "no cycle - linear",
			issues: map[int][]int{
				1: {},
				2: {1},
				3: {2},
			},
			expectError: false,
		},
		{
			name: "no cycle - diamond",
			issues: map[int][]int{
				1: {},
				2: {1},
				3: {1},
				4: {2, 3},
			},
			expectError: false,
		},
		{
			name: "simple cycle A->B->A",
			issues: map[int][]int{
				1: {2},
				2: {1},
			},
			expectError: true,
		},
		{
			name: "longer cycle A->B->C->A",
			issues: map[int][]int{
				1: {3},
				2: {1},
				3: {2},
			},
			expectError: true,
		},
		{
			name: "self-dependency",
			issues: map[int][]int{
				1: {1},
			},
			expectError: true,
		},
		{
			name:        "empty graph",
			issues:      map[int][]int{},
			expectError: false,
		},
		{
			name: "no dependencies",
			issues: map[int][]int{
				1: {},
				2: {},
				3: {},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := detector.CheckForCycles(tt.issues)
			if tt.expectError && err == nil {
				t.Error("expected error for cycle, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestDeduplicateDeps(t *testing.T) {
	detector := NewDependencyDetector(nil, nil, "auto")

	tests := []struct {
		name      string
		deps      []int
		selfIssue int
		expected  []int
	}{
		{
			name:      "remove duplicates",
			deps:      []int{1, 2, 1, 3, 2},
			selfIssue: 99,
			expected:  []int{1, 2, 3},
		},
		{
			name:      "remove self reference",
			deps:      []int{1, 2, 3},
			selfIssue: 2,
			expected:  []int{1, 3},
		},
		{
			name:      "empty list",
			deps:      []int{},
			selfIssue: 1,
			expected:  nil,
		},
		{
			name:      "all self references",
			deps:      []int{5, 5, 5},
			selfIssue: 5,
			expected:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.deduplicateDeps(tt.deps, tt.selfIssue)
			if len(result) != len(tt.expected) {
				t.Errorf("expected length %d, got %d", len(tt.expected), len(result))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("expected %d at index %d, got %d", tt.expected[i], i, v)
				}
			}
		})
	}
}
