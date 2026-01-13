# DevStreamlineBot

A bot that automates code review assignments in GitLab. It integrates with VK Teams to receive commands and send notifications, and with GitLab to manage merge request reviewers.

## Features

- **Auto-reviewer assignment**: Automatically assigns reviewers to new merge requests using weighted random selection based on recent workload
- **Label-based reviewers**: Configure different reviewer pools for specific labels (e.g., backend team for `backend` label)
- **SLA tracking**: Track review and fix times with configurable SLAs, excluding weekends and holidays
- **Review digests**: Send periodic summaries of pending reviews to chat
- **Vacation management**: Mark users as on vacation to exclude them from reviewer selection

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
| `start_time` | Optional. Only process MRs created after this date (YYYY-MM-DD) |

## Bot Commands

Add the bot to a VK Teams chat and use these commands:

### Core Commands

| Command | Description |
|---------|-------------|
| `/subscribe <repo_id>` | Subscribe chat to GitLab project notifications |
| `/unsubscribe <repo_id>` | Unsubscribe from a project |
| `/reviewers user1,user2` | Set default reviewer pool for subscribed repos |
| `/reviewers` | Clear default reviewers |
| `/reviews` | List your pending reviews |
| `/reviews <username>` | List pending reviews for a specific user |
| `/send_digest` | Send immediate review digest to chat |
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
