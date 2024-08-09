package main

import (
	"fmt"
	"main/src/sources"
	"main/src/utils"
)

func SyncUser(gitlab *GitLab, dufs *Dufs, sourceCfg ConfigRepo, groupCfg ConfigGroup, source sources.Source) {
	pageNum := 0
	count := 1

	results, err := source.Paginate(groupCfg.Username, pageNum)
	for {
		if err != nil {
			fmt.Println(err)
			break
		}

		if len(results) == 0 {
			break
		}

		for _, remote := range results {
			if gitlab.isReservedName(remote.Name) {
				fmt.Printf("Skipping repository %s: reserved name\n", remote.Name)
				continue
			}

			if groupCfg.Skip != nil && *groupCfg.Skip >= count {
				fmt.Printf("Skipping repository %s: from --skip\n", remote.Name)
				continue
			}

			// Find configuration for that repo
			cfg := sourceCfg // Default to source's config
			if len(groupCfg.Repositories) > 0 {
				cf := groupCfg.GetConfig(remote.Name)
				if cf != nil {
					cfg = cf.ConfigRepo

					if *cf.Exclude {
						fmt.Printf("Skipping repository %s: from --exclude\n", remote.Name)
						continue
					}
				} else {
					fmt.Printf("Skipping repository %s: from --include-only\n", remote.Name)
					continue
				}
			}

			fmt.Printf("\n%d. Evaluating repository %s\n", count, remote.Name)
			prj := NewProject(gitlab, dufs, *groupCfg.GitLabGroupID, source, groupCfg.Username, remote, cfg)
			if err := SyncRepo(prj); err != nil {
				fmt.Println(err)
			}

			count++
		}

		pageNum++
		results, err = source.Paginate(groupCfg.Username, pageNum)
	}
}

func SyncRepo(prj *Project) error {
	repoID, err := prj.RetrieveExistingRepo()
	if err != nil {
		return err
	}

	// Sync repository
	if repoID == -1 {
		fmt.Println("- Importing new repository in GitLab...")
		repoID, err = prj.Import()
		if err != nil {
			return err
		}
		fmt.Println("- Importing new repository in GitLab with project ID:", repoID)

		fmt.Println("- Create 'original_url' attribute with value:" + prj.SourceRepository.URL)
		err = prj.SetOriginalURL()
		if err != nil {
			return err
		}

		fmt.Println("- Waiting for repository import to finish...")
		err = prj.LockUntilImport()
		if err != nil {
			return err
		}

		protectedBranches, err := prj.GetProtectedBranches()
		fmt.Printf("- Found %d protected branches\n", len(protectedBranches))
		if err != nil {
			return err
		}

		fmt.Println("  - Unprotecting branches...")
		for _, branch := range protectedBranches {
			fmt.Printf("    - Unprotecting %s...\n", branch)
			err = prj.UnprotectBranch(branch)
			if err != nil {
				return err
			}
		}
	} else {
		fmt.Println("- Repository already exists in GitLab with project ID:", repoID)
		fmt.Println("- Cloning repository from GitLab...")
		if err := prj.CloneFromSource(); err != nil {
			return err
		}

		fmt.Println("- Adding GitLab as a remote repository..")
		if err := prj.AddRemoteToRepo(); err != nil {
			return err
		}

		fmt.Println("- Pushing branches to GitLab...")
		branches, err := prj.GetBranches()
		if err != nil {
			return err
		}

		fmt.Printf("  - Found %d branches\n", len(branches))
		for _, branch := range branches {
			fmt.Printf("  - Pushing %s...\n", branch)
			if err := prj.PushBranch(branch); err != nil {
				return err
			}
		}

		fmt.Println("- Pushing tags to GitLab...")
		if err := prj.PushAllTags(); err != nil {
			return err
		}
	}

	// Sync WiKi
	if !*prj.Config.Wiki.Exclude {
		fmt.Println("- Checking for source Wiki...")
		wikiPrj := prj.GetWikiProject()
		if err := wikiPrj.CloneFromSource(); err == nil {
			fmt.Println("  - Found remote Wiki, syncing...")
			if err := wikiPrj.AddRemoteToRepo(); err != nil {
				return err
			}

			fmt.Println("  - Pushing branches to GitLab...")
			branches, err := wikiPrj.GetBranches()
			if err != nil {
				return err
			}

			fmt.Printf("    - Found %d branches\n", len(branches))
			for _, branch := range branches {
				fmt.Printf("    - Pushing %s...\n", branch)
				if err := wikiPrj.PushBranch(branch); err != nil {
					return err
				}
			}

			fmt.Println("  - Pushing tags to GitLab...")
			if err := wikiPrj.PushAllTags(); err != nil {
				return err
			}
		}
	}

	// Sync Releases
	if !*prj.Config.Releases.Exclude {
		fmt.Println("- Fetching source releases...")
		releases, err := prj.Source.FetchReleases(prj.SourceUsername, prj.SourceRepository.Name)
		if err != nil {
			return err
		}

		fmt.Printf("  - Found %d releases\n", len(releases))
		for _, release := range releases {
			fmt.Printf("  - Evaluating release %s...\n", release.TagName)
			exists, err := prj.ReleaseExists(release.TagName)
			if err != nil {
				return err
			}

			if exists {
				fmt.Printf("    - Release %s already exists, skipping...\n", release.TagName)
				continue
			}

			fmt.Printf("    - Release %s does not exist, creating...\n", release.TagName)
			if err := prj.CreateRelease(release); err != nil {
				return err
			}

			if !*prj.Config.Releases.Assets.Exclude {
				fmt.Printf("    - Found %d assets\n", len(release.Assets))
				for _, asset := range release.Assets {
					fmt.Printf("    - Downloading asset: %s\n", asset.Name)
					if err := utils.DownloadAsset(asset.BrowserDownloadUrl, "/tmp/git-backup/"+asset.Name); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}
