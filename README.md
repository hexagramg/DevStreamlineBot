# DevStreamlineBot

A bot designed to streamline development workflows by automating code review admissions in GitLab. When added to VK Teams groups, it monitors specified GitLab projects and manages reviewer assignments for merge requests. The bot supports three core commands:

- `/subscribe 123` - Subscribe to GitLab project with ID 123
- `/unsubscribe 123` - Unsubscribe from a project
- `/reviewers a.afansyiev,b.ivanov` - Set team members as default reviewers

When a new merge request is created (not in draft status), the bot automatically notifies the chat and updates the merge request with reviewer information, helping teams manage their code review process more efficiently. 

## Getting Started

### Prerequisites

- Go installed on your machine
- Access credentials for required services

### Setup

1. Clone this repository
    ```bash
    git clone https://github.com/hexagramg/DevStreamlineBot.git
    cd DevStreamlineBot
    ```

2. Copy the configuration example file
    ```bash
    cp config/config-example.json config.json
    ```

3. Edit the configuration file with your URLs and tokens
    ```bash
    nano config.json
    ```

### Running the Project

Execute the bot using:
```bash
go run main.go
```

## Configuration

The `config.json` file needs to contain your personal API tokens and service URLs. Make sure not to commit this file to version control.
