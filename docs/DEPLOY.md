# Build and Deploy Guide

This guide explains how to build DevStreamlineBot using Docker and deploy it to a remote server.

## Prerequisites

- Docker installed locally
- SSH access to the target server
- Target server running Linux (amd64 architecture)

## Build

Build the static binary using Docker:

```bash
# Build the Docker image
docker build -t dsbot-builder .

# Extract the binary from the container
docker create --name tmp dsbot-builder >/dev/null
docker cp tmp:/usr/local/bin/devstreamlinebot .
docker rm tmp
```

This produces a static `devstreamlinebot` binary for linux/amd64.

## Deploy

Copy the binary to your remote server:

```bash
scp devstreamlinebot user@your-server:/path/to/destination/
```

Example:

```bash
scp devstreamlinebot deploy@192.168.1.100:/opt/devstreamlinebot/
```

## Remote Setup

On the remote server:

```bash
# Make the binary executable (if needed)
chmod +x /path/to/devstreamlinebot

# Create config file
cp config-example.yaml config.yaml
# Edit config.yaml with your settings

# Run the application
./devstreamlinebot
```

## One-Liner

Build and deploy in one command:

```bash
docker build -t dsbot-builder . && \
docker create --name tmp dsbot-builder >/dev/null && \
docker cp tmp:/usr/local/bin/devstreamlinebot . && \
docker rm tmp && \
scp devstreamlinebot user@your-server:/path/to/destination/
```
