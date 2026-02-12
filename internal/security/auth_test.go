package security

import (
	"bytes"
	"log"
	"testing"
)

func TestIsAuthorized_AllowedUser(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)

	if !IsAuthorized([]string{"alice", "bob"}, "alice", logger) {
		t.Error("expected allowed user to be authorized")
	}
}

func TestIsAuthorized_CaseInsensitive(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)

	if !IsAuthorized([]string{"Alice"}, "alice", logger) {
		t.Error("expected case-insensitive match")
	}
}

func TestIsAuthorized_NotInList(t *testing.T) {
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	if IsAuthorized([]string{"alice", "bob"}, "eve", logger) {
		t.Error("expected user not in list to be unauthorized")
	}
	if logBuf.String() == "" {
		t.Error("expected unauthorized attempt to be logged")
	}
}

func TestIsAuthorized_EmptyListAllowsAll(t *testing.T) {
	logger := log.New(&bytes.Buffer{}, "", 0)

	if !IsAuthorized(nil, "anyone", logger) {
		t.Error("expected empty allowed list to authorize all users")
	}
	if !IsAuthorized([]string{}, "anyone", logger) {
		t.Error("expected empty allowed list to authorize all users")
	}
}

func TestIsAuthorized_NilLogger(t *testing.T) {
	if IsAuthorized([]string{"alice"}, "eve", nil) {
		t.Error("expected unauthorized with nil logger")
	}
}
