# Workflow Phases

Ultra Engineer processes issues through a series of phases, each tracked via labels on the issue.

## Phase Overview

| Phase | Label | Description |
|-------|-------|-------------|
| `new` | (none) | Initial state, no processing started |
| `questions` | `phase:questions` | Clarifying Q&A phase |
| `planning` | `phase:planning` | Implementation planning |
| `approval` | `phase:approval` | Awaiting user approval of plan |
| `implementing` | `phase:implementing` | Active code implementation |
| `review` | `phase:review` | Review cycles and refinement |
| `completed` | `phase:completed` | Successfully completed |
| `failed` | `phase:failed` | Failed or aborted |

## Phase Details

### New

**Trigger**: Issue has the trigger label (e.g., `ai-implement`) but no phase label.

**Actions**:
1. Initialize state with new session ID
2. Detect dependencies from issue text
3. Check for blocking dependencies
4. Transition to `questions` phase

**User Interaction**: None required.

### Questions

**Label**: `phase:questions`

**Actions**:
1. Claude analyzes the issue
2. Generates clarifying questions (if needed)
3. Posts questions as a comment
4. Waits for user response

**User Interaction**: Answer the questions in a comment. The Q&A may go through multiple rounds (`QARound` tracks this).

**Transition**: When Claude determines enough context is gathered, moves to `planning`.

### Planning

**Label**: `phase:planning`

**Actions**:
1. Claude creates an implementation plan
2. Plan considers Q&A history and issue details
3. Posts plan as a comment for review
4. Transitions to `approval`

**User Interaction**: None required during this phase.

**State**: `PlanVersion` tracks plan iterations if replanning is needed.

### Approval

**Label**: `phase:approval`

**Actions**:
1. Waits for user to approve or reject the plan
2. Monitors for approval/rejection comments

**User Interaction**:
- Approve: Comment with approval (e.g., "LGTM", "approved", thumbs up reaction)
- Reject: Comment with feedback for a new plan
- If rejected, returns to `planning` with feedback

**Transition**: On approval, moves to `implementing`.

### Implementing

**Label**: `phase:implementing`

**Actions**:
1. Clone repository to sandbox
2. Create feature branch
3. Claude implements the plan
4. Commit changes
5. Push branch
6. Create pull request

**State Tracking**:
- `BranchName`: Working branch
- `PRNumber`: Created PR number

**Transition**: After PR creation, moves to `review`.

### Review

**Label**: `phase:review`

**Actions**:
1. Claude reviews the implementation
2. Makes refinements based on review
3. Pushes updates to PR
4. Repeats for configured number of cycles

**Configuration**: `claude.review_cycles` sets the number of review iterations (default: 5).

**State**: `ReviewIteration` tracks current iteration.

**CI Monitoring** (if enabled):
- Wait for CI to complete
- Attempt to fix CI failures
- `CIFixAttempts` tracks fix attempts

**Transition**: After review cycles complete (and CI passes if enabled), moves to `completed`.

### Completed

**Label**: `phase:completed`

**Actions**:
1. Merge PR (if `auto_merge: true`)
2. Post completion comment
3. Remove trigger label

**Final State**: Issue processing is finished successfully.

### Failed

**Label**: `phase:failed`

**Causes**:
- Unrecoverable error during processing
- Dependency cycle detected
- Manual abort via CLI
- Max retry attempts exceeded
- Plan rejection without replan

**State**: `Error` and `FailureReason` contain details.

**Recovery**: Remove `phase:failed` label and add trigger label to retry.

## State Fields

The workflow state persists these fields in the issue body:

| Field | Type | Description |
|-------|------|-------------|
| `SessionID` | string | Unique identifier for this processing session |
| `CurrentPhase` | Phase | Current workflow phase |
| `LastUpdated` | time.Time | Timestamp of last state update |
| `LastCommentID` | int64 | ID of last processed comment |
| `Error` | string | Error message if any |
| `QAHistory` | []QAEntry | History of Q&A exchanges |
| `QARound` | int | Current Q&A round number |
| `PlanVersion` | int | Version of the implementation plan |
| `ReviewIteration` | int | Current review iteration count |
| `PRNumber` | int | Associated pull request number |
| `BranchName` | string | Working branch name |
| `LastPRCommentTime` | time.Time | For PR comment ordering |
| `CIFixAttempts` | int | Number of CI fix attempts |
| `LastCIStatus` | string | Last observed CI status |
| `CIWaitStartTime` | time.Time | When CI waiting started |
| `DependsOn` | []int | Issue numbers this depends on |
| `BlockedBy` | []int | Issues currently blocking this |
| `FailureReason` | string | Reason for failure (e.g., "dependency_cycle") |

## Label Management

Labels are managed automatically:

- **Adding**: New phase label added on transition
- **Removing**: Old phase labels removed on transition
- **Trigger label**: Removed on completion or failure

The `Labels` type in `internal/state/state.go` handles label transitions.

## Dependency Handling

### Detection

Dependencies are detected from issue text using patterns:
- `depends on #123`
- `after #123`
- `requires #123`
- `blocked by #123`
- `waiting for #123` / `waiting on #123`

### Blocking Behavior

1. On entering `new` phase, dependencies are detected
2. If dependencies exist, `BlockedBy` is populated with incomplete ones
3. Issue is skipped during polling while blocked
4. When blocking issues complete, `BlockedBy` is updated
5. Issue becomes eligible when `BlockedBy` is empty

### Cycle Detection

Before processing, the orchestrator checks for cycles:
- Uses depth-first search on dependency graph
- If cycle found, all issues in cycle are marked `failed`
- `FailureReason` set to "dependency_cycle"
- Error message includes cycle path (e.g., "#1 -> #2 -> #1")

### Overrides

Skip dependency detection:
- Add `no-dependencies` label to the issue
- Include `/no-deps` in the issue body

## User Interaction Points

| Phase | Interaction | Required |
|-------|-------------|----------|
| Questions | Answer clarifying questions | Yes |
| Approval | Approve or reject plan | Yes |
| Review | Provide feedback (optional) | No |
| Failed | Decide to retry or close | Optional |

## Progress Reporting

When `progress.enabled: true`, Ultra Engineer posts progress updates:

- Phase transitions
- Q&A rounds
- Plan versions
- Review iterations
- CI status changes

Updates are debounced by `progress.debounce_interval` (default: 60s) to avoid comment spam. Critical milestones force immediate updates regardless of debounce.
