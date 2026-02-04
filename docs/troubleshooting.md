# Troubleshooting Guide

This guide covers common issues and solutions when using Ultra Engineer.

## Common Issues

### Issue Not Being Processed

**Symptoms:**
- Issue has the trigger label but nothing happens
- No progress comments appear

**Possible Causes:**

1. **Missing trigger label**
   - Verify the issue has the exact label specified in `trigger_label` config
   - Default is `ai-implement`

2. **Daemon not running**
   - Check if the daemon process is active
   - Verify it's monitoring the correct repository

3. **Issue in wrong state**
   - Check if issue already has a `phase:failed` label
   - Remove the failed label and re-add trigger label to retry

4. **Blocked by dependencies**
   - Check if issue depends on other issues
   - Use `ultra-engineer status --repo owner/repo --issue N` to see status
   - Look for `BlockedBy` in the output

**Solution:**
```bash
# Check daemon logs
ultra-engineer daemon -v --repo owner/repo

# Check specific issue status
ultra-engineer status --repo owner/repo --issue 123
```

### Authentication Errors

**Symptoms:**
- "401 Unauthorized" or "403 Forbidden" errors
- "Authentication required" messages

**For GitHub:**
```bash
# Check gh CLI authentication
gh auth status

# Re-authenticate if needed
gh auth login

# Or use a personal access token
export GITHUB_TOKEN=your_token
```

**For Gitea:**
- Verify the token in configuration is valid
- Check token has required permissions (read/write for issues and repos)
- Ensure Gitea URL is correct (no trailing slash)

### Claude CLI Not Found

**Symptoms:**
- "command not found: claude"
- "Claude invocation failed"

**Solution:**
1. Verify Claude CLI is installed: `which claude`
2. If not in PATH, specify full path in config:
   ```yaml
   claude:
     command: /full/path/to/claude
   ```
3. Check Claude CLI is properly configured: `claude --version`

### Rate Limiting

**Symptoms:**
- "Rate limit exceeded" errors
- Processing slows or pauses

**Solution:**
- Configure longer retry intervals:
  ```yaml
  retry:
    rate_limit_retry: 10m
  ```
- Reduce concurrency:
  ```yaml
  concurrency:
    max_per_repo: 2
    max_total: 5
  ```

### Timeout Errors

**Symptoms:**
- "Context deadline exceeded"
- Implementations cut off mid-work

**Solution:**
- Increase Claude timeout:
  ```yaml
  claude:
    timeout: 45m
  ```
- For CI monitoring, increase CI timeout:
  ```yaml
  ci:
    timeout: 45m
  ```

### Dependency Cycle Detected

**Symptoms:**
- Issue marked as failed
- Error message: "dependency cycle detected: #1 -> #2 -> #1"

**Solution:**
1. Review the cycle in the error message
2. Break the cycle by:
   - Removing one dependency reference
   - Adding `no-dependencies` label to one issue
   - Adding `/no-deps` to one issue body

### Merge Conflicts

**Symptoms:**
- PR cannot be merged
- Error: "merge conflict"
- `FailureReason: merge_conflict`

**Solution:**
1. Manually resolve conflicts on the branch
2. Push resolved changes
3. Re-run Ultra Engineer or wait for next poll

### CI Failures

**Symptoms:**
- PR stuck in review phase
- Multiple CI fix attempts logged

**Causes:**
1. Complex failures Claude can't fix
2. Flaky tests
3. Environment-specific issues

**Solution:**
- Check CI logs for the actual failure
- If unfixable automatically, set `ci.max_fix_attempts: 0` and fix manually
- Consider disabling CI waiting for problematic repositories:
  ```yaml
  ci:
    wait_for_ci: false
  ```

## Debugging

### Enable Verbose Logging

```bash
ultra-engineer daemon -v --repo owner/repo
```

### Check Log File

If configured:
```yaml
log_file: /var/log/ultra-engineer.log
```

Review with:
```bash
tail -f /var/log/ultra-engineer.log
```

### Inspect State

View the raw state stored in issue body:
1. Edit the issue
2. Look for the HTML comment: `<!-- ultra-engineer-state ... -->`
3. Parse the JSON to see current state

### Test Single Issue

Instead of running daemon, test a specific issue:
```bash
ultra-engineer run -v --repo owner/repo --issue 123
```

### Check Provider Connectivity

**GitHub:**
```bash
gh api user
gh api repos/owner/repo
```

**Gitea:**
```bash
curl -H "Authorization: token YOUR_TOKEN" https://gitea.example.com/api/v1/user
```

## FAQ

### Can I process the same issue multiple times?

Yes. Remove the `phase:completed` or `phase:failed` label and add the trigger label again. The state will be reset.

### How do I skip dependency detection for one issue?

Add the `no-dependencies` label to the issue, or include `/no-deps` anywhere in the issue body.

### Why isn't my issue being auto-merged?

Check:
1. `defaults.auto_merge` is `true` in config
2. PR passes all required checks
3. No merge conflicts
4. Repository branch protection allows merging

### How do I stop processing an issue?

```bash
ultra-engineer abort --repo owner/repo --issue 123
```

Or manually:
1. Remove the trigger label
2. Add `phase:failed` label

### Can I use Ultra Engineer with private repositories?

Yes, ensure your authentication token has access to the private repository:
- GitHub: Token needs `repo` scope
- Gitea: Token needs read/write access to the repository

### How do I change the number of review cycles?

```yaml
claude:
  review_cycles: 3  # Default is 5
```

### What happens if the daemon crashes?

The daemon is stateless. Just restart it:
- State is preserved in issue bodies
- Processing will resume from the last known phase

### How do I monitor multiple organizations?

Use multiple `--repo` flags:
```bash
ultra-engineer daemon \
  --repo org1/repo1 \
  --repo org1/repo2 \
  --repo org2/repo1
```

### Can I customize the prompts sent to Claude?

Currently, prompts are hardcoded in `internal/claude/prompts.go`. To customize, modify the source and rebuild.

## Getting Help

If you can't resolve an issue:

1. Check existing [GitHub Issues](https://github.com/sallandpioneers/ultra-engineer/issues)
2. Enable verbose logging and collect logs
3. Open a new issue with:
   - Ultra Engineer version
   - Configuration (redact tokens)
   - Error messages
   - Steps to reproduce
