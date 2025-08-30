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
- **HuggingFace**

**Supported features** per platform:

|                        | GitHub | GitLab | HuggingFace |
|------------------------|--------|--------|-------------|
| Branches               | ✅      | ✅      | ✅           |
| Tags                   | ✅      | ✅      | ✅           |
| Wiki                   | ✅      | ✅      | ✅           |
| Releases               | ✅      | ✅      | ✅           |
| Assets                 | ✅      | ✅      | ✅           |
| Archived status check  | ✅      | ✅      |             |
| Avatar                 |        | ✅      |             |
| Include / Exclude      | ✅      | ✅      | ✅           |
| Visibility filter      | ✅      | ✅      | ✅           |
| Regex on name          | ✅      | ✅      | ✅           |
| Description check      | ✅      | ✅      |             |
| License check          | ✅      | ✅      |             |
| Topics check           | ✅      |        |             |
| Language check         | ✅      |        |             |
| Pages check            | ✅      |        |             |
| Discussions check      | ✅      |        |             |
| Issues check           | ✅      |        |             |
| Forked check           | ✅      |        |             |
| Stars check            | ✅      |        |             |
| Watchers check         | ✅      |        |             |
| Forks check            | ✅      |        |             |
| Branches               | ✅      | ✅      | ✅           |
| Tags check             | ✅      | ✅      | ✅           |
| Size check             | ✅      |        |             |
| Open issues check      | ✅      |        |             |
| Creation date check    | ✅      |        |             |
| Last update date check | ✅      |        |             |

## Setup

Populate the `config.json` file (`jsonc` is also supported), see [config.example.jsonc](./config.example.jsonc).

Use the `docker compose` command to create and start the container.

```shell
docker compose up --build -d
```

## Contribution

Contributions are welcome! Please feel free to submit a pull request or file an issue for improvements, features, or bug
fixes.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## TODO
- Allow gitlab to scrape without a token (public requests)
- Add go-gitea as a source
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