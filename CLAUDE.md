# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

DevStreamlineBot is a Go application that automates code review assignments in GitLab. It integrates with VK Teams (via bot-golang) to receive commands and send notifications, and with GitLab to manage merge request reviewers.

## Build & Run Commands

```bash
# Run directly
go run main.go

# Build binary
go build -o devstreamlinebot main.go

# Build via Docker (produces static linux/amd64 binary)
docker build -t dsbot-builder .
docker create --name tmp dsbot-builder >/dev/null
docker cp tmp:/usr/local/bin/devstreamlinebot .
docker rm tmp
```

Note: Requires CGO_ENABLED=1 for SQLite support.

## Configuration

Copy `config/config-example.yaml` to `config.yaml` in the project root and fill in:
- `gitlab.base_url`, `gitlab.token`, `gitlab.poll_interval`
- `vk.base_url`, `vk.token`
- `database.dsn` (SQLite file path)

## Code Style

### Comments

- **Never write obvious comments** that just restate what the code does
- **Only add comments for**:
  - Complex algorithms or non-obvious logic
  - Function/method docstrings explaining purpose and edge cases
  - The "why" behind design decisions, not the "what"
- **Never include in comments**:
  - Task descriptions or requirements
  - Difficulty assessments
  - Implementation progress notes
  - Anything unrelated to the code logic itself

**Good comment examples:**
- Algorithm explanations with enumerated steps
- Edge case documentation
- Design rationale for custom types/patterns

**Avoid:**
- `x = 5 // set x to 5`
- `// TODO: implement this feature`
- `// This was a complex task`

## Architecture

### Core Flow

1. **Startup (main.go)**: Loads config, initializes SQLite via GORM, creates rate-limited GitLab client, fetches all accessible projects, starts polling loops and consumers.

2. **Polling Layer (polling/)**:
   - `vk.go`: Long-polls VK Teams API for messages, upserts Chat/VKUser, emits VKEvent to channel
   - `mrs.go`: Polls GitLab for open MRs on subscribed repos, syncs to DB via `syncGitLabMRToDB()`
   - `repos.go`: Polls and syncs repository metadata
   - `users.go`: Fetches missing GitLab user emails

3. **Consumer Layer (consumers/)**:
   - `vk_command_consumer.go`: Processes VK messages for slash commands (/subscribe, /unsubscribe, /reviewers, /reviews, /send_digest, /get_mr_info)
   - `mr_reviewer_consumer.go`: Auto-assigns reviewers to new MRs using weighted random selection based on recent workload, notifies subscribed chats
   - `review_digest_consumer.go`: Sends periodic review digests

### Data Model (models/models.go)

Key entities with GORM:
- `Repository` - GitLab projects (GitlabID unique)
- `User` - GitLab users (GitlabID unique, Email indexed, OnVacation bool)
- `MergeRequest` - GitLab MRs with many-to-many relations to Reviewers, Approvers, Labels
- `Chat` - VK Teams chats
- `VKUser` - VK Teams users (UserID is email)
- `RepositorySubscription` - Links Chat to Repository for notifications
- `PossibleReviewer` - Links Repository to User for default reviewer pool
- `LabelReviewer` - Links Repository + Label to User for label-specific reviewer pools
- `RepositorySLA` - SLA settings per repository (ReviewDuration, FixesDuration, AssignCount)
- `Holiday` - Non-working days per repository (excluded from SLA calculations)
- `MRComment` - Tracked comments on MRs (resolvable/resolved status)
- `MRAction` - Timeline of MR events for state tracking

### Key Patterns

- **User Identity Linking**: VK user IDs are emails, matched to GitLab users via `User.Email` field
- **Reviewer Selection**: Uses label-priority cascade with weighted random selection (see below)
- **Rate Limiting**: GitLab client uses custom `RateLimitedTransport` (5 req/s, burst 10)
- **MR Sync**: `syncGitLabMRToDB()` handles full upsert of MR with all associations (author, assignee, labels, reviewers, approvers)
- **Discussion Sync**: `syncMRDiscussions()` tracks comments and their resolved status for state derivation

### Reviewer Selection Algorithm

The reviewer assignment uses a label-priority cascade with the following rules:

1. **If MR has labels with configured label reviewers:**
   - Group label reviewers by label name
   - Pick exactly 1 reviewer from each label group (weighted by inverse workload)
   - **No reuse**: Once picked, a user is removed from all remaining pools
   - **Can exceed minimum**: If MR has more labels than AssignCount, still pick one from each
   - If total < AssignCount: pick additional from combined remaining label reviewers + default pool

2. **If no label reviewers available:**
   - Pick AssignCount reviewers from default pool (`PossibleReviewer`)

3. **Exclusions**: MR author and users on vacation are always excluded from selection

### MR State Machine

MR states are derived dynamically based on DB data (not stored as a field):

| State | Condition |
|-------|-----------|
| `merged` | MR.State == "merged" |
| `closed` | MR.State == "closed" |
| `draft` | MR.Draft == true |
| `on_fixes` | Has unresolved resolvable comments |
| `on_review` | Default: has reviewers, no unresolved comments |

**State transition time** is determined from:
- `merged`: MR.MergedAt
- `closed`: MR.ClosedAt
- `draft`: Most recent ActionDraftToggled with metadata `{"draft":true}`
- `on_fixes`: GitlabCreatedAt of first unresolved comment
- `on_review`: Latest of: last comment resolved, draft unmarked, reviewer assigned, or MR created

**Working time** calculation excludes weekends and configured holidays (stored in `Holiday` table).

## Bot Commands

### Core Commands
- `/subscribe <repo_id> [--force]` - Subscribe chat to GitLab repo notifications. Copies settings (reviewers, SLA, holidays) from other repos in the same chat. Use `--force` to take over a repo owned by another chat.
- `/unsubscribe <repo_id>` - Unsubscribe from repo
- `/reviewers user1,user2` - Set default reviewer pool for subscribed repos
- `/reviewers` - Clear default reviewers
- `/reviews [username]` - List pending reviews for user
- `/send_digest` - Send immediate review digest
- `/get_mr_info <path!iid>` - Get MR details (e.g., `intdev/myapp!123`)

### Reviewer Management
- `/label_reviewers <label> user1,user2,...` - Set label-specific reviewers
- `/label_reviewers <label>` - Clear reviewers for a label
- `/label_reviewers` - List all label-reviewer mappings
- `/assign_count <N>` - Set minimum reviewer count for subscribed repos (default: 1)
- `/vacation <username>` - Toggle vacation status for a user

### SLA & Scheduling
- `/sla` - Show current SLA settings
- `/sla review <duration>` - Set review SLA (e.g., `48h`, `2d`, `1w`)
- `/sla fixes <duration>` - Set fixes SLA (time for author to address comments)
- `/holidays` - List configured holidays
- `/holidays date1 date2 ...` - Add holidays (format: DD.MM.YYYY)
- `/holidays remove date1 date2 ...` - Remove specific holidays
