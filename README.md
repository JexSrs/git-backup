# Git Backup

A go script to retrieve the public repositories from various sources and sync them to a GitLab or Gitea instance.

## Features

This tool enhances repository management across multiple platforms by synchronizing essential information.
With the appropriate access token, it can also fetch and sync private repositories the user has access to.

The following features are available:

- Sync all branches
- Sync all tags
- Sync wiki
- Sync releases and their assets
- Mark as archived
- Fetch subgroup/repository avatar (available only from GitLab sources).
- Set the original repository url in the CI/CD variables
- Create the destination owner when it is missing (Gitea only, opt-in via `create_owner`)
- Retry failed page fetches and repository syncs with exponential backoff (opt-in via `config.retries`)
- Include only / Exclude repositories
- Filters based on
    - Visibility & archived status
    - Regex on `name`
    - Existence of `description`
    - Checking the values of `license`, `topics` and `language`
    - `Pages`, `discussions` and `issues` being enabled
    - If it is forked
    - Min/max checks on `stars`, `watchers`, `forks`, `branches`, `tags`, `size` and `open issues`
    - Min/max checks on `creation` and `last updated` dates

**Supported Platforms**
This tool supports synchronization with the following sources:

- **GitHub**
- **GitLab**
- **GitLab (self-hosted)**
- **Gitea / Forgejo** (self-hosted, Codeberg, gitea.com)
- **LavaForge**
- **HuggingFace**

**Supported features** per platform:

|                        | GitHub | GitLab | HuggingFace | Gitea / LavaForge |
|------------------------|--------|--------|-------------|-------------------|
| Branches               | ✅      | ✅      | ✅           | ✅                 |
| Tags                   | ✅      | ✅      | ✅           | ✅                 |
| Wiki                   | ✅      | ✅      | ✅           | ✅                 |
| Releases               | ✅      | ✅      | ✅           | ✅                 |
| Assets                 | ✅      | ✅      | ✅           | ✅                 |
| Archived status check  | ✅      | ✅      |             | ✅                 |
| Avatar                 |        | ✅      |             | ✅                 |
| Include / Exclude      | ✅      | ✅      | ✅           | ✅                 |
| Visibility filter      | ✅      | ✅      | ✅           | ✅                 |
| Regex on name          | ✅      | ✅      | ✅           | ✅                 |
| Description check      | ✅      | ✅      |             | ✅                 |
| License check          | ✅      | ✅      |             |                   |
| Topics check           | ✅      |        |             |                   |
| Language check         | ✅      |        |             |                   |
| Pages check            | ✅      |        |             |                   |
| Discussions check      | ✅      |        |             |                   |
| Issues check           | ✅      |        |             |                   |
| Forked check           | ✅      |        |             | ✅                 |
| Stars check            | ✅      |        |             |                   |
| Watchers check         | ✅      |        |             |                   |
| Forks check            | ✅      |        |             |                   |
| Branches               | ✅      | ✅      | ✅           | ✅                 |
| Tags check             | ✅      | ✅      | ✅           | ✅                 |
| Size check             | ✅      |        |             |                   |
| Open issues check      | ✅      |        |             |                   |
| Creation date check    | ✅      |        |             |                   |
| Last update date check | ✅      |        |             |                   |

## Setup

Populate the `config.json` file (`jsonc` is also supported), see [config.example.jsonc](./config.example.jsonc).

Build and run locally with `make` (by default the binary reads `config.jsonc` from the working directory, so run it from the repo root):

```shell
make build   # compile to ./bin/git-backup
make run     # build and run
make help    # list all targets
```

To use a configuration file from another location, pass `-c` or `--config`. This also lets you run several
instances side by side, each with its own configuration:

```shell
./bin/git-backup --config /path/to/other-config.jsonc
./bin/git-backup -c ./configs/work.jsonc
make run ARGS="-c ./configs/work.jsonc"
```

When `--config` is given, only that file is read; if it does not exist the program exits with an error.
Without it, the binary looks in the working directory for `config.json`, `configuration.json`, `config.jsonc`
or `configuration.jsonc`, in that order.

### Gitea, Forgejo and LavaForge sources

Sources whose `id` starts with `gitea` talk to any Gitea or Forgejo instance (self-hosted, gitea.com, Codeberg) and
need a `base_url`. Sources whose `id` starts with `lavaforge` use the same implementation with `base_url` defaulting to
`https://lavaforge.org`. In both cases `auth.token` is an access token of the instance and is required to see private
repositories; the group's `username` may be either a user or an organisation.

### Retries

Transient failures (for example the destination not being able to reach the source while importing) abort the run
by default. Set `retries` inside `config` to retry a failed page fetch or repository sync that many times before
giving up. Like every other `config` key it can be set globally, per source, per group and per repository; the most
specific value wins and the default is `0`.

Attempts back off exponentially: 2 seconds before the first retry, then 5, then 10, doubling after that and capped at
2 minutes between attempts.

```jsonc
{
    "config": { "retries": 3 }, // global
    "sources": [{ "id": "github", "config": { "retries": 5 } }], // overrides global for github groups
    "groups": [{
        "source": "github", "username": "octo",
        "config": { "retries": 0 }, // overrides the source for this group
        "repositories": [{ "name": "flaky", "retries": 10 }] // overrides the group for one repository
    }]
}
```

Or use the `docker compose` command to create and start the container.

```shell
docker compose up --build -d
```

## Contribution

Contributions are welcome! Please feel free to submit a pull request or file an issue for improvements, features, or bug
fixes.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## TODO
- Check latest commit's hash for existing entries to skip in case of no updates
- Enhance filters
    - releases (min, max)
    - all issues (min, max)
    - closed issues (min, max)
    - all pull_requests (min, max)
    - open pull_requests (min, max)
    - closed pull_requests (min, max)
    - commits (min, max)
- Update avatar's on groups (GitLab)
- Change subgroup if changed from source (GitLab)