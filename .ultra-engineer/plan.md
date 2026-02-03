# Implementation Plan: Add a Simple Greeting Function

## Overview

Create a simple Go function `Greet` in `internal/greeting/greeting.go` that takes a name string parameter and returns a greeting message.

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/greeting/greeting.go` | Create | New file containing the `Greet` function |
| `internal/greeting/greeting_test.go` | Create | Unit tests for the `Greet` function |

## Step-by-Step Approach

1. **Create the package directory structure**
   - Create `internal/greeting/` directory

2. **Implement the Greet function**
   - Create `internal/greeting/greeting.go`
   - Define package `greeting`
   - Implement `Greet(name string) string` function that returns a greeting message in the format "Hello, {name}!"

3. **Add unit tests**
   - Create `internal/greeting/greeting_test.go`
   - Test with a normal name (e.g., "Alice" → "Hello, Alice!")
   - Test with an empty string (returns "Hello, !" - accepted behavior for simplicity)
   - Test with special characters (e.g., "O'Brien" → "Hello, O'Brien!")

## Testing Approach

1. **Unit Tests**
   - Run `go test ./internal/greeting/...` to execute the unit tests
   - Verify the function returns the expected greeting format

2. **Build Verification**
   - Run `go build ./...` to ensure the code compiles without errors

## Design Notes

- **Package location**: Using `internal/greeting/` to maintain consistency with the existing codebase structure (all other packages are in `internal/`).
- **Edge case behavior**: Empty strings are passed through as-is (returns "Hello, !"). This keeps the function simple and predictable. If different behavior is needed, the caller can validate input before calling.
