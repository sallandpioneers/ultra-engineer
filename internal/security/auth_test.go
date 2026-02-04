package security

import (
	"bytes"
	"context"
	"log"
	"testing"

	"github.com/anthropics/ultra-engineer/internal/providers"
)

func TestIsAuthorized_CollaboratorIsAuthorized(t *testing.T) {
	mock := providers.NewMockProvider()
	mock.SetCollaborator("owner/repo", "collaborator", true)

	logger := log.New(&bytes.Buffer{}, "", 0)

	authorized, err := IsAuthorized(context.Background(), mock, "owner/repo", "collaborator", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !authorized {
		t.Error("expected collaborator to be authorized")
	}
}

func TestIsAuthorized_NonCollaboratorIsNotAuthorized(t *testing.T) {
	mock := providers.NewMockProvider()
	// Don't set the user as collaborator - default is false

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	authorized, err := IsAuthorized(context.Background(), mock, "owner/repo", "outsider", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorized {
		t.Error("expected non-collaborator to not be authorized")
	}

	// Check that the unauthorized attempt was logged
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Error("expected unauthorized attempt to be logged")
	}
}

func TestIsAuthorized_IssueAuthorWhoIsNotCollaboratorIsNotAuthorized(t *testing.T) {
	mock := providers.NewMockProvider()
	// Add an issue created by "issueAuthor" who is NOT a collaborator
	mock.AddIssue("owner/repo", &providers.Issue{
		Number: 1,
		Title:  "Test Issue",
		Author: "issueAuthor",
	})
	// Explicitly set issueAuthor as NOT a collaborator
	mock.SetCollaborator("owner/repo", "issueAuthor", false)

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	authorized, err := IsAuthorized(context.Background(), mock, "owner/repo", "issueAuthor", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorized {
		t.Error("expected issue author who is not a collaborator to not be authorized")
	}
}

func TestIsAuthorized_NilLoggerDoesNotPanic(t *testing.T) {
	mock := providers.NewMockProvider()
	// Don't set as collaborator - should log but not panic with nil logger

	authorized, err := IsAuthorized(context.Background(), mock, "owner/repo", "user", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorized {
		t.Error("expected unauthorized with nil logger")
	}
}

func TestIsAuthorized_ExplicitlySetCollaboratorFalse(t *testing.T) {
	mock := providers.NewMockProvider()
	// Explicitly set user as NOT a collaborator
	mock.SetCollaborator("owner/repo", "user", false)

	logger := log.New(&bytes.Buffer{}, "", 0)

	authorized, err := IsAuthorized(context.Background(), mock, "owner/repo", "user", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorized {
		t.Error("expected user explicitly set as non-collaborator to not be authorized")
	}
}
