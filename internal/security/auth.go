package security

import (
	"log"
	"strings"
)

// IsAuthorized checks if a user is in the allowed users list.
// If the allowed list is empty, all users are authorized.
func IsAuthorized(allowedUsers []string, username string, logger *log.Logger) bool {
	if len(allowedUsers) == 0 {
		return true
	}

	for _, u := range allowedUsers {
		if strings.EqualFold(u, username) {
			return true
		}
	}

	if logger != nil {
		logger.Printf("Unauthorized access attempt: user %s is not in allowed_users", username)
	}
	return false
}
