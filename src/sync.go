package main

import (
	"fmt"
	"log"
	"main/src/configuration"
	"main/src/dest"
	"main/src/sources"
	"main/src/utils"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// retrySleep waits between retry attempts; tests replace it so they never block.
var retrySleep = time.Sleep

// withRetries runs fn and, when it fails, retries it as many times as cfg allows,
// backing off between attempts and reporting every failed attempt.
func withRetries(cfg configuration.ConfigRepo, what string, fn func() error) error {
	retries := cfg.RetryCount()
	return utils.Retry(retries, retrySleep, func(attempt int, wait time.Duration, err error) {
		fmt.Printf("- %s failed (attempt %d of %d): %v\n", what, attempt, retries+1, err)
		fmt.Printf("- Retrying in %s...\n", wait)
	}, fn)
}

func SyncUser(dst map[string]dest.Destination, groupCfg configuration.ConfigGroup, source sources.Source) {
	fmt.Println("\n================================================================")
	fmt.Printf("Evaluating group %s from %s\n", groupCfg.Username, groupCfg.Source)
	fmt.Println("================================================================")

	count := 1
	needs := statsNeeded(groupCfg.Filter)
	statsUnavailableReported := false

	paginate := func(prev *sources.PaginationResponse) (*sources.PaginationResponse, error) {
		var page *sources.PaginationResponse
		err := withRetries(groupCfg.Config, "Fetching repositories", func() error {
			var err error
			page, err = source.Paginate(groupCfg.Username, groupCfg, prev)
			return err
		})
		return page, err
	}

	result, err := paginate(nil)
	for {
		if err != nil {
			fmt.Println(err)
			break
		}

		if len(result.Repositories) == 0 {
			break
		}

		for _, remote := range result.Repositories {
			if groupCfg.Skip != nil && *groupCfg.Skip >= count {
				fmt.Printf("Skipping repository %s: from --skip\n", remote.Name)
				count++
				continue
			}

			if groupCfg.IncludeOnly != nil && !utils.ContainsIgnoreCase(groupCfg.IncludeOnly, remote.Name) {
				fmt.Printf("Skipping repository %s: from --include-only\n", remote.Name)
				continue
			}

			if groupCfg.Exclude != nil && utils.ContainsIgnoreCase(groupCfg.Exclude, remote.Name) {
				fmt.Printf("Skipping repository %s: from --exclude\n", remote.Name)
				continue
			}

			if skipFromFilter(remote, groupCfg.Filter) {
				fmt.Printf("Skipping repository %s: from --filter\n", remote.Name)
				continue
			}

			// Count filters need per-repository lookups, so they run only after the cheap filters passed.
			if needs.Any() {
				stats, err := source.FetchStats(groupCfg.Username, remote, needs)
				remote.Stats = stats
				if err != nil {
					if errors.Is(err, sources.ErrStatsUnavailable) {
						if !statsUnavailableReported {
							fmt.Printf("Count filters are not applied to this group: %v\n", err)
							statsUnavailableReported = true
						}
					} else {
						fmt.Printf("Could not read the statistics of repository %s, count filters not applied to it: %v\n", remote.Name, err)
					}
				}

				if reason := statsFilterReason(remote.Stats, groupCfg.Filter); reason != "" {
					fmt.Printf("Skipping repository %s: from --filter (%s)\n", remote.Name, reason)
					continue
				}
			}

			fmt.Printf("\n%d. Evaluating repository %s\n", count, remote.Name)
			cfg := groupCfg.GetConfig(remote.Name)
			if err := withRetries(cfg, "Syncing repository "+remote.Name, func() error {
				return SyncRepo(dst, source, remote, cfg, groupCfg)
			}); err != nil {
				log.Fatal(err)
			}

			count++
		}

		result, err = paginate(result)
	}
}

func SyncRepo(
	dst map[string]dest.Destination,
	source sources.Source,
	remote sources.SourceRepository,
	config configuration.ConfigRepo,
	gConfig configuration.ConfigGroup,
) error {
	// Match destination
	gitDst := dst[*config.Destination]
	storageDst := dst[*config.Releases.Assets.Destination]

	dstID := gitDst.GetIdentification()

	created, err := gitDst.EnsureOwner(gConfig)
	if err != nil {
		return errors.Wrap(err, "failed to ensure repository owner exists")
	}
	if created {
		fmt.Printf("- Owner '%s' did not exist in destination, created it as an organization\n", gConfig.GiteaUsername)
	}

	fmt.Println("- Checking repository...")
	repo, err := gitDst.RetrieveExistingRepo(gConfig, remote)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve existing repo")
	}

	if repo == nil || !repo.FinishedMiration {
		if repo == nil {
			fmt.Println("- Repository does not exist in destination")
			fmt.Println("- Importing...")
			repo, err = gitDst.ImportRepository(gConfig, config, remote, source)
			if err != nil {
				return errors.Wrap(err, "failed to import project")
			}
			fmt.Println("- Imported new repository with id:", repo.ID)
		} else {
			fmt.Println("- Repository already exists in destination with id", repo.ID)
		}

		if !remote.Empty {
			fmt.Println("  - Migration has not finished")
			fmt.Println("- Waiting for repository import to finish...")
			if err := gitDst.LockUntilImport(repo, func(status string) {
				fmt.Println("  - Current import status:", status)
			}); err != nil {
				return errors.Wrap(err, "failed to read import status")
			}
		}

		fmt.Println("- Setting 'original_url' attribute:", remote.URL)
		if err := gitDst.SetOriginalUrl(repo, remote.URL); err != nil {
			return errors.Wrap(err, "failed to set original url")
		}

		protectedBranches, err := gitDst.GetProtectedBranches(repo)
		if err != nil {
			return errors.Wrap(err, "failed to get list of protected branches")
		}
		fmt.Printf("- Found %d protected branches\n", len(protectedBranches))

		if len(protectedBranches) != 0 {
			fmt.Println("- Unprotecting branches...")
		}

		for _, branch := range protectedBranches {
			fmt.Printf("  - Unprotecting %s...\n", branch)
			if err := gitDst.UnprotectBranch(repo, branch); err != nil {
				return errors.Wrapf(err, "failed to unprotect branch %s", branch)
			}
		}
	} else {
		fmt.Println("- Repository already exists in destination with id", repo.ID)
		if *gConfig.Filter.OnlyNew {
			fmt.Println("- Only new is enabled, skipping...")
			return nil
		}

		// Un-archive project to sync branches/releases (in case it was archived and re-archived from last time)
		if err := gitDst.ChangeArchivedState(repo, false); err != nil {
			return errors.Wrap(err, "failed to change project archive state")
		}

		fmt.Println("- Comparing source and destination branches and tags...")
		upToDate, differences, err := repo.IsUpToDate(source, gitDst.RemoteURL(repo), *dstID.Config)
		if err != nil {
			fmt.Println("  - Could not compare, syncing anyway:", err)
		}

		if upToDate {
			fmt.Println("  - Destination already has every branch and tag of the source, skipping clone")
		} else {
			if err == nil {
				printRefDifferences(differences)
			}

			fmt.Println("- Cloning repository from source...")
			if err := repo.CloneFromSource(source, *dstID.Config); err != nil {
				return errors.Wrap(err, "failed to clone source")
			}

			defer func() {
				if err := repo.Prune(); err != nil {
					fmt.Println(errors.Wrap(err, "failed to prune project"))
				}
			}()

			fmt.Println("- Adding destination as a remote repository..")
			if err := gitDst.AddRemoteToRepo(repo); err != nil {
				return errors.Wrap(err, "failed to add destination as a remote repository")
			}

			fmt.Println("- Pushing branches...")
			branches, err := repo.GetLocalBranches()
			if err != nil {
				return errors.Wrap(err, "failed to retrieve local branches")
			}

			fmt.Printf("  - Found %d branches\n", len(branches))
			for _, branch := range branches {
				fmt.Printf("  - Pushing %s...\n", branch)
				if err := repo.PushLocalBranch(branch, dstID.ID); err != nil {
					return errors.Wrapf(err, "failed to sync branch %s", branch)
				}
			}

			fmt.Println("- Pushing tags...")
			if err := repo.PushAllTags(dstID.ID); err != nil {
				return errors.Wrap(err, "failed to sync tags")
			}
		}
	}

	if *config.FetchAvatar && dstID.Repository.Avatar {
		fmt.Println("- Checking for avatar...")
		if remote.Avatar != nil {
			fmt.Println("  - Downloading...")
			ext := utils.ExtractExtension(*remote.Avatar)
			avatarBuffer, err := utils.DownloadAsset(*remote.Avatar)
			if err != nil {
				return errors.Wrap(err, "failed to download avatar")
			}

			if ext == "ico" {
				avatarBuffer, err = utils.ConvertICOtoPNG(avatarBuffer)
				if err != nil {
					return errors.Wrap(err, "failed to convert avatar: ico to png")
				}

				ext = "png"
			}

			fmt.Println("  - Uploading avatar...")
			if err := gitDst.ChangeAvatar(repo, avatarBuffer, ext); err != nil {
				return errors.Wrap(err, "failed to upload avatar")
			}
		}
	}

	if !*config.Wiki.Exclude && dstID.Repository.Wiki {
		fmt.Println("- Checking for source Wiki...")
		wikiRepo := gitDst.GetWikiProject(repo, gConfig, source)
		if wikiRepo != nil {
			if err := wikiRepo.CloneFromSource(source, *dstID.Config); err == nil {
				defer func() {
					if err := wikiRepo.Prune(); err != nil {
						fmt.Println(errors.Wrap(err, "wiki: failed to prune project"))
					}
				}()

				fmt.Println("  - Found Wiki, adding remote...")
				if err := gitDst.AddRemoteToRepo(wikiRepo); err != nil {
					return errors.Wrap(err, "wiki: failed to add destination as a remote repository")
				}

				fmt.Println("  - Creating new Wiki project if it does not exist...")
				if err := gitDst.CreateWiki(repo, gConfig); err != nil {
					return errors.Wrap(err, "wiki: failed to create project")
				}

				fmt.Println("  - Pushing branches to destination...")
				branches, err := wikiRepo.GetLocalBranches()
				if err != nil {
					return errors.Wrap(err, "wiki: failed to retrieve branches")
				}

				fmt.Printf("    - Found %d branches\n", len(branches))
				for _, branch := range branches {
					fmt.Printf("    - Pushing %s...\n", branch)
					if err := wikiRepo.PushLocalBranch(branch, dstID.ID); err != nil {
						return errors.Wrapf(err, "wiki: failed to sync branch %s", branch)
					}
				}

				fmt.Println("  - Pushing tags...")
				if err := wikiRepo.PushAllTags(dstID.ID); err != nil {
					return errors.Wrap(err, "wiki: failed to sync tags")
				}
			}
		} else {
			fmt.Println("  - Source does not support WiKi")
		}
	}

	if !*config.Releases.Exclude && dstID.Repository.Releases {
		fmt.Println("- Fetching source releases...")
		releases, err := source.FetchReleases(gConfig.Username, remote)
		if err != nil {
			return errors.Wrap(err, "releases: failed to fetch")
		}

		if releases != nil {
			fmt.Printf("  - Found %d releases\n", len(releases))
			for _, release := range releases {
				fmt.Printf("  - Evaluating release %s...\n", release.TagName)
				if exists, err := gitDst.ReleaseExists(repo, release.TagName); err != nil || exists {
					if err != nil {
						return errors.Wrapf(err, "release: failed to check existence")
					} else {
						fmt.Println("    - Release already exists, skipping...")
						continue
					}
				}

				fmt.Println("    - Release does not exist, creating...")
				if err := gitDst.CreateRelease(repo, release); err != nil {
					return errors.Wrap(err, "release: failed to create")
				}

				fmt.Printf("    - Found %d assets\n", len(release.Assets))
				for _, asset := range release.Assets {
					fmt.Printf("    - Evaluating asset: %s\n", asset.Name)

					// If asset is not downloaded, then set the original asset url
					assetURL := asset.URL
					if !*config.Releases.Assets.Exclude {
						fmt.Println("      - Downloading...")

						assetBuffer, err := utils.DownloadAsset(asset.URL)
						if err != nil {
							return errors.Wrap(err, "asset: failed to download: "+asset.URL)
						}

						assetShouldBeUploaded := true

						maxSize := *config.Releases.Assets.MaxSize
						if maxSize != "none" {
							maxSizeBytes := utils.ConvertToBytes(maxSize)

							fmt.Printf("      - Size: %s\n", utils.ConvertFromBytes(assetBuffer.Len()))
							if assetBuffer.Len() >= int(maxSizeBytes) {
								fmt.Printf("      - Asset %s exceeds the maximum size of %s\n", asset.Name, maxSize)
								assetShouldBeUploaded = false
							}
						}

						// Upload asset
						if assetShouldBeUploaded {
							fmt.Println("      - Uploading asset to storage...")

							assetURL = fmt.Sprintf("/%s/repositories/repo_%s/tag_%s/%s",
								dstID.ID,
								repo.ID,
								strings.ReplaceAll(release.TagName, "/", "-"),
								strings.ReplaceAll(asset.Name, "/", "-"),
							)

							url, err := storageDst.UploadFile(assetBuffer, assetURL)
							if err != nil {
								return errors.Wrap(err, "asset: failed to upload")
							}

							assetURL = url
						}
					}

					// Link asset
					fmt.Println("      - Linking asset to release...")
					if err := gitDst.LinkAsset(repo, release, asset.Name, assetURL); err != nil {
						return errors.Wrap(err, "asset: failed to link")
					}

					fmt.Println("      - Done")
				}
			}
		}
	}

	if remote.Archived {
		fmt.Println("- Changing repository state to archived")
		if err := gitDst.ChangeArchivedState(repo, true); err != nil {
			return errors.Wrap(err, "failed to change project state")
		}
	}

	return nil
}

// printRefDifferences lists the branches and tags that need syncing, capped so large repositories stay readable.
func printRefDifferences(differences []dest.RefDifference) {
	const maxLines = 10

	fmt.Printf("  - %d branches/tags need syncing\n", len(differences))
	for i, difference := range differences {
		if i == maxLines {
			fmt.Printf("    - ... and %d more\n", len(differences)-maxLines)
			break
		}
		fmt.Println("    -", difference)
	}
}

// statsNeeded derives which repository statistics the count filters require. A bound that cannot
// exclude anything (min of zero or less and no max) needs nothing, so the default config costs no lookups.
func statsNeeded(filter configuration.ConfigFilter) sources.StatsNeeds {
	return sources.StatsNeeds{
		Releases: boundsCanExclude(filter.Releases),
		Commits:  boundsCanExclude(filter.Commits),
		Issues: sources.StateNeeds{
			All:    boundsCanExclude(filter.Issues.All),
			Open:   boundsCanExclude(filter.Issues.Open),
			Closed: boundsCanExclude(filter.Issues.Closed),
		},
		PullRequests: sources.StateNeeds{
			All:    boundsCanExclude(filter.PullRequests.All),
			Open:   boundsCanExclude(filter.PullRequests.Open),
			Closed: boundsCanExclude(filter.PullRequests.Closed),
		},
	}
}

func boundsCanExclude(bounds configuration.ConfigFilterMinMax[int]) bool {
	return (bounds.Min != nil && *bounds.Min > 0) || bounds.Max != nil
}

// statsFilterReason explains why the count filters exclude the repository, or returns "" when they do
// not. Bounds are inclusive. Unknown counts never exclude: a source that cannot provide a statistic is
// simply not filtered on it.
func statsFilterReason(stats sources.RepositoryStats, filter configuration.ConfigFilter) string {
	checks := []struct {
		name   string
		value  *int
		bounds configuration.ConfigFilterMinMax[int]
	}{
		{"releases", stats.Releases, filter.Releases},
		{"commits", stats.Commits, filter.Commits},
		{"issues", stats.Issues.All, filter.Issues.All},
		{"open issues", stats.Issues.Open, filter.Issues.Open},
		{"closed issues", stats.Issues.Closed, filter.Issues.Closed},
		{"pull requests", stats.PullRequests.All, filter.PullRequests.All},
		{"open pull requests", stats.PullRequests.Open, filter.PullRequests.Open},
		{"closed pull requests", stats.PullRequests.Closed, filter.PullRequests.Closed},
	}

	for _, check := range checks {
		if check.value == nil {
			continue
		}
		if check.bounds.Min != nil && *check.value < *check.bounds.Min {
			return fmt.Sprintf("%s %d below min %d", check.name, *check.value, *check.bounds.Min)
		}
		if check.bounds.Max != nil && *check.value > *check.bounds.Max {
			return fmt.Sprintf("%s %d above max %d", check.name, *check.value, *check.bounds.Max)
		}
	}
	return ""
}

func skipFromFilter(remote sources.SourceRepository, filter configuration.ConfigFilter) bool {
	if filter.Visibility != nil {
		if remote.Private && !utils.ContainsIgnoreCase(filter.Visibility, "private") {
			return true
		}

		if !remote.Private && !utils.ContainsIgnoreCase(filter.Visibility, "public") {
			return true
		}
	}

	if filter.Archived != nil {
		if *filter.Archived != remote.Archived {
			return true
		}
	}

	if filter.Empty != nil {
		if *filter.Empty != remote.Empty {
			return true
		}
	}

	if filter.HasDescription != nil {
		if *filter.HasDescription != (remote.Description != nil && len(*remote.Description) != 0) {
			return true
		}
	}

	if filter.Forked != nil {
		if *filter.Forked != remote.Forked {
			return true
		}
	}

	return false
}
