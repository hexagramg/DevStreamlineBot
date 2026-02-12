# DevStreamlineBot

A bot that automates code review assignments in GitLab. It integrates with VK Teams to receive commands and send notifications, and with GitLab to manage merge request reviewers.

## Features

- **Auto-reviewer assignment**: Automatically assigns reviewers to new merge requests using weighted random selection based on recent workload
- **Label-based reviewers**: Configure different reviewer pools for specific labels (e.g., backend team for `backend` label)
- **SLA tracking**: Track review and fix times with configurable SLAs, excluding weekends and holidays
- **Review digests**: Send periodic summaries of pending reviews to chat
- **Personal daily digests**: Get personalized daily action items sent to DMs (weekdays only, skips holidays)
- **Vacation management**: Mark users as on vacation to exclude them from reviewer selection
- **Auto-release branches**: Automatically create release branches, retarget MRs, and maintain release MR descriptions with included changes
- **Feature release branches**: Create and manage feature-specific release branches in parallel with regular releases
- **Deploy tracking**: Monitor GitLab deploy jobs and receive notifications on deployment status changes
- **Jira integration**: Extract Jira task IDs from branch names or MR titles for linking
- **Release-ready labels**: Mark MRs as ready for release with dedicated labels
- **Release notifications**: Subscribe chats to get notified when MRs are marked release-ready
- **DM notifications**: Receive personal notifications for MR state changes, approvals, and reviewer updates

## Getting Started

### Prerequisites

- Go 1.21+ (CGO_ENABLED=1 required for SQLite)
- Access to GitLab API
- VK Teams bot token

### Setup

1. Clone this repository
    ```bash
    git clone https://github.com/hexagramg/DevStreamlineBot.git
    cd DevStreamlineBot
    ```

2. Copy the configuration example file
    ```bash
    cp config/config-example.yaml config.yaml
    ```

3. Edit the configuration file with your settings
    ```bash
    nano config.yaml
    ```

### Running

Run directly:
```bash
go run main.go
```

Or build a binary:
```bash
go build -o devstreamlinebot main.go
./devstreamlinebot
```

### Docker Build

Build a static linux/amd64 binary using Docker:
```bash
docker build -t dsbot-builder .
docker create --name tmp dsbot-builder >/dev/null
docker cp tmp:/usr/local/bin/devstreamlinebot .
docker rm tmp
```

## Configuration

The `config.yaml` file contains all settings. Do not commit this file with real credentials.

```yaml
gitlab:
  base_url: "https://gitlab.example.com"  # GitLab instance URL
  token: "glpat-xxxxxxxxxxxx"             # GitLab API token (read_api scope)
  poll_interval: "1m"                      # How often to poll for MR updates

vk:
  base_url: "https://api.vkteams.example.com"  # VK Teams API URL
  token: "xxxxxxxxxxxx"                         # Bot token from VK Teams

database:
  dsn: "devstreamline.db"  # SQLite database file path

jira:
  base_url: ""  # Optional: Jira instance URL for task linking

# Optional: Override start time for MR processing (format: YYYY-MM-DD)
# If not set, defaults to 2 days before bot startup
# start_time: "2025-01-01"
```

### Config Fields

| Field | Description |
|-------|-------------|
| `gitlab.base_url` | GitLab instance URL (e.g., `https://gitlab.com`) |
| `gitlab.token` | GitLab personal access token with `read_api` scope |
| `gitlab.poll_interval` | Polling interval for MR updates (e.g., `30s`, `1m`, `5m`) |
| `vk.base_url` | VK Teams API base URL |
| `vk.token` | VK Teams bot token |
| `database.dsn` | Path to SQLite database file |
| `jira.base_url` | Optional. Jira instance URL for generating clickable task links in release MR descriptions |
| `start_time` | Optional. Only process MRs created after this date (YYYY-MM-DD) |

## Bot Commands

Add the bot to a VK Teams chat and use these commands:

### Core Commands

| Command | Description |
|---------|-------------|
| `/subscribe <repo_id> [--force]` | Subscribe chat to GitLab project notifications. Use `--force` to take over a repo from another chat |
| `/unsubscribe <repo_id>` | Unsubscribe from a project |
| `/reviewers user1,user2` | Set default reviewer pool for subscribed repos |
| `/reviewers` | Clear default reviewers |
| `/actions [username]` | List pending actions (reviews, fixes, author MRs) for a user |
| `/send_digest` | Send immediate review digest to chat |
| `/daily_digest [+/-N]` | Toggle personal daily digest at 10:00 in your timezone (DM only) |
| `/subscribers` | List all users subscribed to daily digests |
| `/get_mr_info <path!iid>` | Get MR details (e.g., `/get_mr_info group/project!123`) |

### Reviewer Management

| Command | Description |
|---------|-------------|
| `/label_reviewers <label> user1,user2` | Set reviewers for a specific label |
| `/label_reviewers <label>` | Clear reviewers for a label |
| `/label_reviewers` | List all label-reviewer mappings |
| `/assign_count <N>` | Set minimum reviewer count (default: 1) |
| `/vacation <username>` | Toggle vacation status for a user |

### SLA & Scheduling

| Command | Description |
|---------|-------------|
| `/sla` | Show current SLA settings |
| `/sla review <duration>` | Set review SLA (e.g., `48h`, `2d`, `1w`) |
| `/sla fixes <duration>` | Set fixes SLA (time for author to address comments) |
| `/holidays` | List configured holidays |
| `/holidays date1 date2 ...` | Add holidays (format: DD.MM.YYYY) |
| `/holidays remove date1 ...` | Remove specific holidays |

### Label Management

| Command | Description |
|---------|-------------|
| `/add_block_label <label> [#color]` | Add block label(s) to repos (default: #dc143c). MRs with block labels are excluded from auto-retargeting |
| `/add_release_label <label> [#color]` | Add release label to repos (default: #808080). Required for auto-release branches |
| `/add_release_ready_label <label> [#color]` | Add release-ready label (default: #FFD700). Marks MRs as ready for release |
| `/add_feature_release_tag <label> [#color]` | Add feature release label (default: #9370DB). Marks MRs as feature releases |
| `/add_jira_prefix <PREFIX>` | Configure Jira project prefix for task ID extraction from branch names/MR titles |
| `/ensure_label <label> <#color>` | Create label in GitLab if it doesn't exist |

### Release Management

| Command | Description |
|---------|-------------|
| `/auto_release_branch <prefix> : <dev_branch>` | Enable auto-release branches (e.g., `/auto_release_branch release : develop`) |
| `/auto_release_branch` | Disable auto-release branches for subscribed repos |
| `/release_managers user1,user2` | Set release managers for subscribed repos |
| `/release_managers` | List current release managers |
| `/release_subscribe <repo_id>` | Subscribe chat to release notifications (notified when MRs are marked release-ready) |
| `/release_unsubscribe <repo_id>` | Unsubscribe from release notifications |
| `/spawn_branch <project_id or project_name> [custom name]` | Create a new feature release branch with MR. Optional custom name becomes MR title. |

### Deploy Tracking

| Command | Description |
|---------|-------------|
| `/track_deploy <pipeline_job_link> <target_project_id>` | Monitor a GitLab deploy job and send notifications to chats subscribed to the target repo's releases |
| `/untrack_deploy <project_id>` | Remove all deploy tracking rules for a repository |

**Note**: Auto-release branch functionality requires a release label to be configured (`/add_release_label`). Release notifications require a release-ready label (`/add_release_ready_label`). Feature release branches require both a feature release label (`/add_feature_release_tag`) and auto-release config.

## How It Works

### Reviewer Assignment

When a new MR is created (not draft), the bot assigns reviewers using this algorithm:

1. **Label priority**: If MR has labels with configured reviewers, pick one reviewer from each label group
2. **Default pool**: Fill remaining slots from the default reviewer pool
3. **Weighted selection**: Reviewers with fewer recent assignments are more likely to be selected
4. **Exclusions**: MR author and users on vacation are never assigned

### SLA Tracking

The bot tracks time spent in each MR state:
- **on_review**: Waiting for reviewers to approve
- **on_fixes**: Author addressing reviewer comments

Working time excludes weekends and configured holidays.

### DM Notifications

Users receive personal DM notifications for:
- **State changes**: When MRs they're involved in move between states (review ↔ fixes)
- **Fully approved**: When all assigned reviewers have approved an MR
- **Reviewer removal**: When removed as a reviewer from an MR

### Auto-Release Branches

When enabled, the bot automates release branch management:

1. **Branch creation**: Creates a release branch named `{prefix}_{YYYY-MM-DD}_{commit_sha[:6]}` from the dev branch
2. **Release MR**: Creates a merge request with the configured release label, targeting the dev branch
3. **MR retargeting**: Automatically retargets open MRs from the dev branch to the release branch (except blocked MRs)
4. **Description updates**: Keeps the release MR description updated with a list of included MRs:
   ```
   ---
   ## Included MRs
   - [!123 Feature title](https://gitlab.com/...) by @author
   - [!124 Another feature](https://gitlab.com/...) by @developer
   ```
5. **Continuous releases**: When a release MR is merged, a new release branch is automatically created

**Requirements**:
- Repository must have a release label configured (used to identify release MRs)
- Optional: Configure block labels to prevent specific MRs from being retargeted

### Feature Release Branches

For managing feature-specific releases in parallel with regular releases:

1. **Configure feature release label**: Use `/add_feature_release_tag <label>` to set up a label (default color: purple #9370DB)
2. **Create feature branch**: Use `/spawn_branch <project> [custom name]` to create a feature release branch
   - Creates a branch named `feature_release_YYYY-MM-DD_SHA6` from the dev branch
   - Automatically creates an MR targeting the dev branch with the feature release label
   - Optional custom name becomes the MR title (defaults to "Feature Release YYYY-MM-DD")
3. **Description updates**: The bot keeps the feature release MR description updated with included commits
4. **Isolation**: MRs with feature release labels are excluded from regular release retargeting and review digests

**Requirements**: Feature release label configured (`/add_feature_release_tag`) and auto-release config present (`/auto_release_branch`).

### Deploy Tracking

Monitor GitLab CI/CD deploy jobs and receive chat notifications on status changes:

1. **Set up tracking**: Use `/track_deploy <job_url> <target_project_id>` with a link to a GitLab pipeline job
2. **Notifications**: The bot polls for job status changes and sends notifications to chats with release subscriptions for the target repo:
   - Job started running
   - Job succeeded
   - Job failed
   - Job canceled
3. **Remove tracking**: Use `/untrack_deploy <project_id>` to stop monitoring

### Release-Ready Workflow

For teams using a release-ready workflow:

1. **Configure release-ready label**: Use `/add_release_ready_label` to set up a label for marking MRs ready for release
2. **Subscribe to notifications**: Use `/release_subscribe` in chats that should receive release notifications
3. **Mark MRs ready**: When an MR is ready for release, add the release-ready label in GitLab
4. **Receive notifications**: Subscribed chats are notified when MRs are marked release-ready
