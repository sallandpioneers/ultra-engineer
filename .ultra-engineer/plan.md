# Implementation Plan: Add Logging to File

## Overview

Add the ability to log output to a file in addition to stdout. This feature will allow users to specify a log file path via CLI flag and/or configuration, and all log messages will be written to both stdout and the specified file simultaneously.

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/ultra-engineer/main.go` | Modify | Add `--log-file` CLI flag and helper function for logger setup |
| `internal/config/config.go` | Modify | Add `LogFile` config option |
| `cmd/ultra-engineer/daemon.go` | Modify | Use new logger setup with file output |
| `cmd/ultra-engineer/run.go` | Modify | Use new logger setup with file output |

Note: `status.go` and `abort.go` do not use loggers (they use `fmt` for output), so they do not need modification.

## Step-by-Step Approach

1. **Update configuration**
   - Add `LogFile string` field to `Config` struct in `internal/config/config.go`
   - Add YAML tag `log_file` for config file support

2. **Add CLI flag and helper function**
   - Add `--log-file` persistent flag in `cmd/ultra-engineer/main.go`
   - Add a `setupLogger(logFile string, verbose bool) (*log.Logger, func(), error)` helper function in `main.go` that:
     - Creates a multi-writer that writes to both stdout and the file
     - Returns a cleanup function to close the file handle
     - Handles the case where no log file is specified (stdout only)
     - Creates parent directories if they don't exist (using `os.MkdirAll`)
     - Uses file permissions `0644` for the log file
   - Flag takes precedence over config file setting

3. **Update command files**
   - Modify `daemon.go` and `run.go` to use the new logger setup
   - Call cleanup function via `defer` immediately after obtaining it (before signal handling setup)
   - The `defer` ensures cleanup runs on both normal exit and panic; signal handling already uses context cancellation which will cause the main function to return and trigger deferred cleanup

## Testing Approach

1. **Unit Tests**
   - Create `cmd/ultra-engineer/main_test.go` with tests for `setupLogger`:
     - Test stdout-only mode (empty log file path) - verify returned logger writes to stdout
     - Test file+stdout mode - use a temp file and verify content is written to it
     - Test parent directory creation - use a nested temp path
     - Test cleanup function closes file handle - verify file can be removed after cleanup
     - Test invalid path handling - verify graceful fallback to stdout-only

2. **Build Verification**
   - Run `go build ./...` to ensure the code compiles without errors

## Design Notes

- **Simple helper function**: Instead of a separate package, use a helper function in `main.go` since only two commands need it
- **Multi-writer approach**: Using Go's `io.MultiWriter` to write to both stdout and file simultaneously
- **Graceful cleanup**: Return a cleanup function to properly close file handles
- **Directory creation**: Create parent directories with `os.MkdirAll` if they don't exist
- **File permissions**: Use `0644` for log files (readable by all, writable by owner)
- **No rotation**: Keep implementation simple; leave log rotation to external tools like logrotate
- **Same format**: File logs use the same format as stdout for consistency
- **Error handling**: If the log file cannot be opened, log a warning to stderr and continue with stdout only
- **Write errors**: If writing to the log file fails during operation (e.g., disk full), `io.MultiWriter` will return an error which the `log` package handles gracefully by continuing operation; no special handling needed
