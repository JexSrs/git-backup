# Git Backup
A go script to retrieve the public repositories from various sources and sync them to a GitLab instance.

## Features

This tool enhances repository management across multiple platforms by synchronizing essential information.
With the appropriate access token, it can also fetch and sync private repositories the user has access to.

The following features are available:
- Sync all branches
- Sync all tags
- Sync wiki
- Sync releases and their assets
- Mark as archived
- Sync repository avatar (available only from GitLab sources).
- Set the source url in the CI/CD variables
- Include only / Exclude repositories

**Supported Platforms**
This tool supports synchronization with the following sources:
- **GitHub**
- **GitLab**
- **GitLab (self-hosted)**
- **HuggingFace**

## Requirements
1. The GitLab's URL instance (defaults to `https://gitlab.com/`)
2. The GitLab user's token
3. The source user's token
4. The dufs url (for asset uploading, optional)

## Setup

Populate the `config.json` file (`json5` is also supported), see [config.example.json5](./config.example.json5).

Use the `docker compose` command to create and start the container.
```shell
docker compose up --build -d
```
