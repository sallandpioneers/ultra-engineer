package security

import (
	"context"
	"log"

	"github.com/anthropics/ultra-engineer/internal/providers"
)

// IsAuthorized checks if a comment author is authorized to interact with the workflow.
// Only repository collaborators with sufficient permissions (admin, maintain, write, triage)
// are authorized. Non-collaborators cannot interact, even if they created the issue.
//
// If IsCollaborator returns an error, it is logged and the function returns false, nil
// (fail closed). This prevents transient API errors from causing workflow failures while
// still denying access when authorization cannot be verified.
func IsAuthorized(ctx context.Context, provider providers.Provider, repo, commentAuthor string, logger *log.Logger) (bool, error) {
	isCollab, err := provider.IsCollaborator(ctx, repo, commentAuthor)
	if err != nil {
		// Log the error for debugging but fail closed
		if logger != nil {
			logger.Printf("Authorization check failed for user %s on %s: %v (treating as unauthorized)", commentAuthor, repo, err)
		}
		return false, nil
	}

	if !isCollab {
		// Log unauthorized attempts for security monitoring
		if logger != nil {
			logger.Printf("Unauthorized access attempt: user %s is not a collaborator on %s", commentAuthor, repo)
		}
		return false, nil
	}

	return true, nil
}
