# github-to-gitlab-backup
A shell script to retrieve the public repos from a user in GitHub and sync them to a GitLab instance

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
sudo apt install curl jq
```

## How to run

Make file executable:

```shell
chmod a+x script.sh
```

Execute:

```shell
./script.sh --github-username "BitWarden" --gitlab-id "418"
```

or clone specific repositories only:

```shell
# Include specific repositories only
./script.sh --github-username "BitWarden" --gitlab-id "418" --include-only "server clients"
# Exlude specific repositories
./script.sh --github-username "BitWarden" --gitlab-id "418" --exclude "server clients"
# Skip first 10 repositories (ordered by name)
./script.sh --github-username "BitWarden" --gitlab-id "418" --skip 10
```

The option `--exclude` has a higher priority than `--include-only`. If a repository is mentioned in both options, it will be excluded.

The option `--skip` will be executed after filtering with the other two options.
For example for the repostitories `repo1`, `repo2`, `repo3` and `repo4`, by calling the command:
```bash
./script.sh --exclude "repo2" --skip 2
```
we will fetch only `repo4` (`repo1` and `repo3` will be skipped from option `--skip`).