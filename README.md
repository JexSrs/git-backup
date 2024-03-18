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
./script.sh "BitWarden" "418"
```

or clone specific repositories only:

```shell
./script.sh "BitWarden" "418" "server clients"
```