package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupLogger_StdoutOnly(t *testing.T) {
	logger, cleanup, err := setupLogger("", false)
	if err != nil {
		t.Fatalf("setupLogger returned error: %v", err)
	}
	defer cleanup()

	if logger == nil {
		t.Fatal("setupLogger returned nil logger")
	}

	// Verify logger works by writing a message
	logger.Println("test message")
}

func TestSetupLogger_StdoutOnlyVerbose(t *testing.T) {
	logger, cleanup, err := setupLogger("", true)
	if err != nil {
		t.Fatalf("setupLogger returned error: %v", err)
	}
	defer cleanup()

	if logger == nil {
		t.Fatal("setupLogger returned nil logger")
	}
}

func TestSetupLogger_WithFile(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, cleanup, err := setupLogger(logPath, false)
	if err != nil {
		t.Fatalf("setupLogger returned error: %v", err)
	}

	// Write a test message
	testMsg := "test message for file"
	logger.Println(testMsg)

	// Cleanup and verify file was written
	cleanup()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), testMsg) {
		t.Errorf("log file does not contain expected message. Got: %s", content)
	}
}

func TestSetupLogger_CreatesParentDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "dir", "test.log")

	logger, cleanup, err := setupLogger(nestedPath, false)
	if err != nil {
		t.Fatalf("setupLogger returned error: %v", err)
	}
	defer cleanup()

	if logger == nil {
		t.Fatal("setupLogger returned nil logger")
	}

	// Verify directory was created
	dir := filepath.Dir(nestedPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("parent directory was not created: %s", dir)
	}

	// Write a message to verify it works
	logger.Println("test message")
}

func TestSetupLogger_CleanupClosesFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, cleanup, err := setupLogger(logPath, false)
	if err != nil {
		t.Fatalf("setupLogger returned error: %v", err)
	}

	// Write something to ensure file is created
	logger.Println("test")

	// Call cleanup
	cleanup()

	// After cleanup, we should be able to remove the file
	// (this would fail on Windows if the file handle was still open)
	err = os.Remove(logPath)
	if err != nil {
		t.Errorf("failed to remove log file after cleanup (file handle may not be closed): %v", err)
	}
}

func TestSetupLogger_InvalidPath(t *testing.T) {
	// Use /dev/null/invalid which is an invalid path on Unix systems
	// The function should gracefully fall back to stdout-only
	invalidPath := "/dev/null/invalid/path/test.log"

	logger, cleanup, err := setupLogger(invalidPath, false)
	if err != nil {
		t.Fatalf("setupLogger should not return error for invalid path: %v", err)
	}
	defer cleanup()

	if logger == nil {
		t.Fatal("setupLogger returned nil logger")
	}

	// Logger should still work (stdout only)
	logger.Println("test message")
}

func TestSetupLogger_VerboseMode(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, cleanup, err := setupLogger(logPath, true)
	if err != nil {
		t.Fatalf("setupLogger returned error: %v", err)
	}
	defer cleanup()

	if logger == nil {
		t.Fatal("setupLogger returned nil logger")
	}

	// Write a message
	logger.Println("verbose test")
}
