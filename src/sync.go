package main

import (
	"fmt"
	"github.com/pkg/errors"
	"main/src/configuration"
	"main/src/dest"
	"main/src/sources"
	"main/src/utils"
	"strings"
)

func SyncUser(dst map[string]dest.Destination, groupCfg configuration.ConfigGroup, source sources.Source) {
	fmt.Println("\n================================================")
	fmt.Printf("Evaluating group %s from %s\n", groupCfg.Username, groupCfg.Source)
	fmt.Println("================================================")

	count := 1

	result, err := source.Paginate(groupCfg.Username, nil)
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

			fmt.Printf("\n%d. Evaluating repository %s\n", count, remote.Name)
			cfg := groupCfg.GetConfig(remote.Name)
			if err := SyncRepo(dst, source, remote, cfg, groupCfg); err != nil {
				fmt.Println(err)
			}

			count++
		}

		result, err = source.Paginate(groupCfg.Username, result)
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

	fmt.Println("- Checking repository...")
	repo, err := gitDst.RetrieveExistingRepo(gConfig, remote)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve existing repo")
	}

	if repo == nil {
		fmt.Println("- Repository does not exist in gitDst...")
		repo, err = gitDst.ImportRepository(gConfig, remote, source)
		if err != nil {
			return errors.Wrap(err, "failed to import project")
		}
		fmt.Println("- Imported new repository in gitDst with identification:", repo.ID)

		fmt.Println("- Setting 'original_url' attribute with value:", remote.URL)
		if err := gitDst.SetOriginalUrl(repo, remote.URL); err != nil {
			return errors.Wrap(err, "failed to set original url")
		}

		fmt.Println("- Waiting for repository import to finish...")
		if err := gitDst.LockUntilImport(repo); err != nil {
			return errors.Wrap(err, "failed to read import status")
		}

		protectedBranches, err := gitDst.GetProtectedBranches(repo)
		if err != nil {
			return errors.Wrap(err, "failed to get list of protected branches")
		}
		fmt.Printf("- Found %d protected branches\n", len(protectedBranches))

		fmt.Println("  - Unprotecting branches...")
		for _, branch := range protectedBranches {
			fmt.Printf("    - Unprotecting %s...\n", branch)
			if err := gitDst.UnprotectBranch(repo, branch); err != nil {
				return errors.Wrapf(err, "failed to unprotect branch %s", branch)
			}
		}
	} else {
		fmt.Println("- Repository already exists in gitDst with identification", repo.ID)
		fmt.Println("- Cloning repository from source...")
		if err := gitDst.CloneFromSource(repo, source); err != nil {
			return errors.Wrap(err, "failed to clone source")
		}

		defer func() {
			if err := repo.Prune(); err != nil {
				fmt.Println(errors.Wrap(err, "failed to prune project"))
			}
		}()

		fmt.Println("- Adding gitDst as a remote repository..")
		if err := gitDst.AddRemoteToRepo(repo); err != nil {
			return errors.Wrap(err, "failed to add gitDst as a remote repository")
		}

		// Un-archive project to sync branches/releases (in case it was archived and re-archived from last time)
		if err := gitDst.ChangeArchivedState(repo, false); err != nil {
			return errors.Wrap(err, "failed to change project archive state")
		}

		fmt.Println("- Pushing branches to GitLab...")
		branches, err := gitDst.GetLocalBranches(repo)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve local branches")
		}

		fmt.Printf("  - Found %d branches\n", len(branches))
		for _, branch := range branches {
			fmt.Printf("  - Pushing %s...\n", branch)
			if err := gitDst.PushLocalBranch(repo, branch); err != nil {
				return errors.Wrapf(err, "failed to sync branch %s", branch)
			}
		}

		fmt.Println("- Pushing tags...")
		if err := gitDst.PushAllTags(repo); err != nil {
			return errors.Wrap(err, "failed to sync tags")
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

			fmt.Println("  - Uploading avatar to gitDst...")
			if err := gitDst.ChangeAvatar(repo, avatarBuffer, ext); err != nil {
				return errors.Wrap(err, "failed to link avatar in gitDst")
			}

			fmt.Println("  - Done")
		}
	}

	if !*config.Wiki.Exclude && dstID.Repository.Wiki {
		fmt.Println("- Checking for source Wiki...")
		wikiRepo := gitDst.GetWikiProject(repo, gConfig, source)
		if wikiRepo != nil {
			if err := gitDst.CloneFromSource(wikiRepo, source); err == nil {
				defer func() {
					if err := wikiRepo.Prune(); err != nil {
						fmt.Println(errors.Wrap(err, "failed to prune wiki project"))
					}
				}()

				fmt.Println("  - Found remote Wiki, syncing...")
				if err := gitDst.AddRemoteToRepo(wikiRepo); err != nil {
					return errors.Wrap(err, "failed to add GitLab as a remote repository in wiki")
				}

				fmt.Println("  - Pushing branches to gitDst...")
				branches, err := gitDst.GetLocalBranches(wikiRepo)
				if err != nil {
					return errors.Wrap(err, "failed to retrieve branches in wiki")
				}

				fmt.Printf("    - Found %d branches\n", len(branches))
				for _, branch := range branches {
					fmt.Printf("    - Pushing %s...\n", branch)
					if err := gitDst.PushLocalBranch(wikiRepo, branch); err != nil {
						return errors.Wrapf(err, "failed to sync branch %s in wiki", branch)
					}
				}

				fmt.Println("  - Pushing tags...")
				if err := gitDst.PushAllTags(wikiRepo); err != nil {
					return errors.Wrap(err, "failed to sync tags in wiki")
				}
			}
		} else {
			fmt.Println("  - Source does not support WiKi repository...")
		}
	}

	if !*config.Releases.Exclude && dstID.Repository.Releases {
		fmt.Println("- Fetching source releases...")
		releases, err := source.FetchReleases(gConfig.Username, remote)
		if err != nil {
			return errors.Wrap(err, "failed to fetch releases")
		}

		if releases != nil {
			fmt.Printf("  - Found %d releases\n", len(releases))
			for _, release := range releases {
				fmt.Printf("  - Evaluating release %s...\n", release.TagName)
				if exists, err := gitDst.ReleaseExists(repo, release.TagName); err != nil || exists {
					if err != nil {
						return errors.Wrapf(err, "failed to check for release")
					} else {
						fmt.Println("    - Release already exists, skipping...")
						continue
					}
				}

				fmt.Println("    - Release does not exist, creating...")
				if err := gitDst.CreateRelease(repo, release); err != nil {
					return errors.Wrap(err, "failed to create release")
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
							return errors.Wrap(err, "failed to download asset: "+asset.URL)
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
								return errors.Wrap(err, "failed to upload asset")
							}

							assetURL = url
						}
					}

					// Link asset
					fmt.Println("      - Linking asset to GitLab...")
					if err := gitDst.LinkAsset(repo, release.TagName, asset.Name, assetURL); err != nil {
						return errors.Wrap(err, "failed to link asset in gitlab")
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
