# Git Backup
A go script to retrieve the public repositories from various sources and sync them to a GitLab instance.

It will retrieve and sync the following information for each public repo:
- all the branches
- all the tags
- latest releases & assets
- wiki

Supported sources:
- GitHub
- HuggingFace

## Prerequisites
1. The GitLab's URL instance (defaults to `https://gitlab.com/`)
2. The GitLab user's token
3. The source user's token
4. The dufs url (for asset uploading)

## Setup

Populate the `config.json` file, see [config.example.json5](./config.example.json5).

Use the `docker compose` command to create and start the container.
```shell
docker compose up --build -d
```

## Configuration file

See [config.example.json5](./config.example.json5) for how to configure.

## TODOs

1. If repositories' array contains only excluded repositories, then sync all except the mentioned ones
2. Add new source GitLab