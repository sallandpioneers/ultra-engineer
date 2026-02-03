package greeting

import "testing"

func TestGreet(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal name",
			input:    "Alice",
			expected: "Hello, Alice!",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "Hello, !",
		},
		{
			name:     "special characters",
			input:    "O'Brien",
			expected: "Hello, O'Brien!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Greet(tt.input)
			if result != tt.expected {
				t.Errorf("Greet(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
