package main

import (
	"fmt"
	"github.com/pkg/errors"
	"main/src/sources"
	"main/src/utils"
	"os"
	"path/filepath"
	"strings"
)

func SyncUser(gitlab *GitLab, dufs *Dufs, groupCfg ConfigGroup, source sources.Source) {
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
			if gitlab.IsReservedName(remote.Name) {
				fmt.Printf("Skipping repository %s: reserved name\n", remote.Name)
				continue
			}

			if !gitlab.IsValidName(remote.Name) {
				fmt.Printf("Skipping repository %s: invalid name\n", remote.Name)
				continue
			}

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
			prj := NewProject(gitlab, dufs, *groupCfg.GitLabGroupID, source, groupCfg.Username, remote, cfg)
			if err := SyncRepo(prj); err != nil {
				fmt.Println(err)
			}

			// Close project and delete any allocated storage
			if err := prj.Prune(); err != nil {
				fmt.Println(errors.Wrap(err, "failed to prune project"))
			}

			count++
		}

		result, err = source.Paginate(groupCfg.Username, result)
	}
}

func SyncRepo(prj *Project) error {
	repoID, err := prj.RetrieveExistingRepo()
	if err != nil {
		return errors.Wrap(err, "failed to retrieve existing repo")
	}

	// Sync repository
	if repoID == -1 {
		fmt.Println("- Repository does not exist in GitLab...")
		repoID, err = prj.Import()
		if err != nil {
			return errors.Wrap(err, "failed to import project")
		}
		fmt.Println("- Importing new repository in GitLab with project ID:", repoID)

		fmt.Println("- Create 'original_url' attribute with value:", prj.SourceRepository.URL)
		err = prj.SetOriginalURL()
		if err != nil {
			return errors.Wrap(err, "failed to set original url")
		}

		fmt.Println("- Waiting for repository import to finish...")
		err = prj.LockUntilImport()
		if err != nil {
			return errors.Wrap(err, "failed to read import status")
		}

		protectedBranches, err := prj.GetProtectedBranches()
		if err != nil {
			return errors.Wrap(err, "failed to get list of protected branches")
		}
		fmt.Printf("- Found %d protected branches\n", len(protectedBranches))

		fmt.Println("  - Unprotecting branches...")
		for _, branch := range protectedBranches {
			fmt.Printf("    - Unprotecting %s...\n", branch)
			err = prj.UnprotectBranch(branch)
			if err != nil {
				return errors.Wrapf(err, "failed to unprotect branch %s", branch)
			}
		}
	} else {
		fmt.Println("- Repository already exists in GitLab with project ID:", repoID)
		fmt.Println("- Cloning repository from source...")
		if err := prj.CloneFromSource(); err != nil {
			return errors.Wrap(err, "failed to clone source")
		}

		fmt.Println("- Adding GitLab as a remote repository..")
		if err := prj.AddRemoteToRepo(); err != nil {
			return errors.Wrap(err, "failed to add GitLab as a remote repository")
		}

		fmt.Println("- Pushing branches to GitLab...")
		branches, err := prj.GetBranches()
		if err != nil {
			return errors.Wrap(err, "failed to retrieve branches")
		}

		fmt.Printf("  - Found %d branches\n", len(branches))
		for _, branch := range branches {
			fmt.Printf("  - Pushing %s...\n", branch)
			if err := prj.PushBranch(branch); err != nil {
				return errors.Wrapf(err, "failed to sync branch %s", branch)
			}
		}

		fmt.Println("- Pushing tags to GitLab...")
		if err := prj.PushAllTags(); err != nil {
			return errors.Wrap(err, "failed to sync tags")
		}
	}

	// Sync WiKi
	if !*prj.Config.Wiki.Exclude {
		fmt.Println("- Checking for source Wiki...")
		wikiPrj := prj.GetWikiProject()
		if len(wikiPrj.SourceRepository.URL) == 0 {
			fmt.Println("  - Source does not support WiKi repository...")
		} else {
			if err := wikiPrj.CloneFromSource(); err == nil {
				fmt.Println("  - Found remote Wiki, syncing...")
				if err := wikiPrj.AddRemoteToRepo(); err != nil {
					return errors.Wrap(err, "failed to add GitLab as a remote repository in wiki")
				}

				fmt.Println("  - Pushing branches to GitLab...")
				branches, err := wikiPrj.GetBranches()
				if err != nil {
					return errors.Wrap(err, "failed to retrieve branches in wiki")
				}

				fmt.Printf("    - Found %d branches\n", len(branches))
				for _, branch := range branches {
					fmt.Printf("    - Pushing %s...\n", branch)
					if err := wikiPrj.PushBranch(branch); err != nil {
						return errors.Wrapf(err, "failed to sync branch %s in wiki", branch)
					}
				}

				fmt.Println("  - Pushing tags to GitLab...")
				if err := wikiPrj.PushAllTags(); err != nil {
					return errors.Wrap(err, "failed to sync tags in wiki")
				}
			}

			if err := prj.Prune(); err != nil {
				return errors.Wrap(err, "failed to prune wiki project")
			}
		}
	}

	// Sync Releases
	if !*prj.Config.Releases.Exclude {
		fmt.Println("- Fetching source releases...")
		releases, err := prj.Source.FetchReleases(prj.SourceUsername, prj.SourceRepository.Name)
		if err != nil {
			return errors.Wrap(err, "failed to fetch releases")
		}

		if releases == nil {
			fmt.Println("  - Releases are not supported...")
		} else {
			fmt.Printf("  - Found %d releases\n", len(releases))
			for _, release := range releases {
				fmt.Printf("  - Evaluating release %s...\n", release.TagName)
				exists, err := prj.ReleaseExists(release.TagName)
				if err != nil {
					return errors.Wrapf(err, "failed to check for release")
				}

				if exists {
					fmt.Println("    - Release already exists, skipping...")
					continue
				}

				fmt.Println("    - Release does not exist, creating...")
				if err := prj.CreateRelease(release); err != nil {
					return errors.Wrap(err, "failed to create release")
				}

				fmt.Printf("    - Found %d assets\n", len(release.Assets))
				for _, asset := range release.Assets {
					fmt.Printf("    - Evaluating asset: %s\n", asset.Name)

					// If asset is not downloaded, then set the original asset url
					assetURL := asset.BrowserDownloadUrl

					if !*prj.Config.Releases.Assets.Exclude {
						fmt.Println("      - Downloading...")
						assetPath := filepath.Join(prj.GetDir(), "assets__", asset.Name)
						if err := utils.DownloadAsset(asset.BrowserDownloadUrl, assetPath); err != nil {
							return errors.Wrap(err, "failed to download asset")
						}

						assetShouldBeUploaded := true

						maxSize := *prj.Config.Releases.Assets.MaxSize
						if maxSize != "none" {
							maxSizeBytes := utils.ConvertToBytes(maxSize)

							size, err := utils.GetFileSize(assetPath)
							if err != nil {
								return errors.Wrap(err, "failed to stat asset")
							}

							fmt.Printf("      - Size: %s\n", utils.ConvertFromBytes(size))
							if size >= maxSizeBytes {
								fmt.Printf("      - Asset %s exceeds the maximum size of %s\n", asset.Name, maxSize)
								if err := os.Remove(assetPath); err != nil {
									return errors.Wrap(err, "failed to delete asset from local path")
								}

								assetShouldBeUploaded = false
							}
						}

						// Upload asset
						if assetShouldBeUploaded {
							fmt.Println("      - Uploading asset to storage...")

							assetURL = fmt.Sprintf("/gitlab/projects/prj_%d/tag_%s/%s",
								repoID,
								strings.ReplaceAll(release.TagName, "/", "-"),
								strings.ReplaceAll(asset.Name, "/", "-"),
							)

							if err := prj.DestinationStorage.UploadFIle(assetPath, assetURL); err != nil {
								return errors.Wrap(err, "failed to upload asset")
							}

							assetURL = prj.DestinationStorage.URL.JoinPath(assetURL).String()

							// Delete file after upload
							if err := os.Remove(assetPath); err != nil {
								return errors.Wrap(err, "failed to delete asset from local path")
							}
						}
					}

					// Link asset
					fmt.Println("      - Linking asset to GitLab...")
					if err := prj.LinkAsset(release.TagName, asset.Name, assetURL); err != nil {
						return errors.Wrap(err, "failed to link asset in gitlab")
					}

					fmt.Println("      - Done")
				}
			}
		}
	}

	return nil
}
