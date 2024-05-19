# github-to-gitlab-backup
A shell script to retrieve the public repositories from a user in GitHub and sync them to a GitLab instance

It will retrieve and sync the following information for each public repo:
- all the branches
- all the tags
- latest 10 releases & assets (asset retention of latest 10 releases, can be disabled in `delete_asset` function)
- wiki if available

## Prerequisites
- `GITHUB_TOKEN`: the user's GitHub token
- `GITLAB_URL`: the target GitLab instance
- `GITLAB_TOKEN`: the user's token in the target GitLab instance
- `STORAGE_URL`: the [dufs](https://github.com/sigoden/dufs) instance url

All urls should **NOT** end with a slash (/).

Install packages:

```shell
apt-get install curl jq
```

## How to run

First populate the variables at the top of the `script.sh`.

Make file executable:
```shell
chmod a+x script.sh
```

Execute:
```shell
./script.sh --github-username "BitWarden" --gitlab-id "418"
```

The gitlab id must be a group inside GitLab. It is advised for each GitHub username to have a different GitLab group.

### Clone specific repositories:

```shell
# Include specific repositories
./script.sh --github-username "BitWarden" --gitlab-id "418" --include-only "server clients"
# Exlude specific repositories
./script.sh --github-username "BitWarden" --gitlab-id "418" --exclude "server clients"
# Skip first 10 repositories
./script.sh --github-username "BitWarden" --gitlab-id "418" --skip 10
```

The option `--exclude` has a higher priority than `--include-only`. If a repository is mentioned in both options, it will be excluded.

The option `--skip` will be executed before filtering with the other two options.
For example for the repostitories `repo1`, `repo2`, `repo3` and `repo4`, by calling the commands:
```shell
./script.sh --exclude "repo1" --skip 1 # will fetch only repo2, repo3 and repo4.
./script.sh --exclude "repo2 repo4" --skip 2 # will fetch only repo3.
./script.sh --exclude "repo2" --skip 1 # will fetch only repo3 and repo4.

./script.sh --include-only "repo1" --skip 1 # will not fetch anything.
./script.sh --include-only "repo2" --skip 1 # will fetch only repo2.
./script.sh --include-only "repo2 repo3" --skip 2 # will fetch only repo3.
```

### Release and assets per repo

Like `--exlude` and `--include-only` options above, we have a similar functionallity for releases and assets
- `--exclude-releases-for`: to exclude release fetching for specific repositories
- `--inlclude-releases-only-for`: to allow release fetching only for specific repositories
- `--exclude-assets-for`: to exlude assets download for specific repositories
- `--include-assets-only-for`: to allow assets download only for specific repositories

The options `--exclude-assets-for` and `--include-assets-only-for` will set the asset url to the original GitHub asset url for all the excluded assets.

```shell
./script.sh --github-username "BitWarden" --gitlab-id "418" --exclude-releases-for "server"
./script.sh --github-username "BitWarden" --gitlab-id "418" --inlclude-releases-only-for "server clients"
./script.sh --github-username "BitWarden" --gitlab-id "418" --exclude-assets-for "mobile"
./script.sh --github-username "BitWarden" --gitlab-id "418" --include-assets-only-for "server clients"
```

### Assets rules

The size of the assets varies between repositories and releases. If we don't want to fetch assets greater than a specific
size we can use the option `--asset-max-size` (default set to `1M`):
```shell
./script.sh --github-username "BitWarden" --gitlab-id "418" --asset-max-size "10KB" # Do not fetch assets greater than 10 Kilobytes
./script.sh --github-username "BitWarden" --gitlab-id "418" --asset-max-size "5M" # Do not fetch assets greater than 5 Megabytes
./script.sh --github-username "BitWarden" --gitlab-id "418" --asset-max-size "1G" # Do not fetch assets greater than 1 Gugabyte
```

Like the options `--exclude-assets-for` and `--include-assets-only-for`, ecluded assets will have their asset url set to the original GitHub asset url.


To free up space, the script will delete all the asset files from dufs for older releases (currently set to 10).
To change that we can use the option `--delete-release-assets-threshold`:
```shell
./script.sh --github-username "BitWarden" --gitlab-id "418" --delete-release-assets-threshold 15 # Keep the assets only for the 15 latest releases
./script.sh --github-username "BitWarden" --gitlab-id "418" --delete-release-assets-threshold 20 # Keep the assets only for the 20 latest releases
./script.sh --github-username "BitWarden" --gitlab-id "418" --delete-release-assets-threshold 100 # Keep the assets only for the 100 latest releases

./script.sh --github-username "BitWarden" --gitlab-id "418" --delete-release-assets-threshold "none" # Or to disable it, to do not delete any assets
```

The assets that were deleted from dufs will not have their urls replaced in GitLab.

### Wiki fetching

Like `--exlude` and `--include-only` options above, we have a similar functionallity for the WiKi repositories:
- `--exclude-wiki-for`: to exclude WiKi fetching for specific repositories
- `--include-wiki-only-for`: to allow WiKi fetching only for specific repositories

### Multiple instances

The script can have multiple instances for different GitHub users, without affecting each other.
