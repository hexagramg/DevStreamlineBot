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
- `User` - GitLab users (GitlabID unique, Email indexed)
- `MergeRequest` - GitLab MRs with many-to-many relations to Reviewers, Approvers, Labels
- `Chat` - VK Teams chats
- `VKUser` - VK Teams users (UserID is email)
- `RepositorySubscription` - Links Chat to Repository for notifications
- `PossibleReviewer` - Links Repository to User for reviewer pool

### Key Patterns

- **User Identity Linking**: VK user IDs are emails, matched to GitLab users via `User.Email` field
- **Reviewer Selection**: `pickReviewer()` uses inverse-weighted probability based on review counts from past 14 days
- **Rate Limiting**: GitLab client uses custom `RateLimitedTransport` (5 req/s, burst 10)
- **MR Sync**: `syncGitLabMRToDB()` handles full upsert of MR with all associations (author, assignee, labels, reviewers, approvers)

## Bot Commands

- `/subscribe <repo_id>` - Subscribe chat to GitLab repo notifications
- `/unsubscribe <repo_id>` - Unsubscribe from repo
- `/reviewers user1,user2` - Set possible reviewers for subscribed repos
- `/reviewers` - Clear reviewers
- `/reviews [username]` - List pending reviews for user
- `/send_digest` - Send immediate review digest
- `/get_mr_info <path!iid>` - Get MR details (e.g., `intdev/myapp!123`)
