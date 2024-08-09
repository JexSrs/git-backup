package main

import (
	"fmt"
	"main/src/sources"
)

func SyncUser(gitlab *GitLab, configSource ConfigRepo, configRepo ConfigRepository, source sources.Source) {
	pageNum := 0
	count := 1

	results, err := source.Paginate(configRepo.Username, pageNum)
	for {
		if err != nil {
			fmt.Println(err)
			break
		}

		if len(results) == 0 {
			break
		}

		for _, result := range results {
			// Find configuration for that repo
			cfg := configSource // Default to source's config
			exclude := false
			if len(configRepo.Repositories) > 0 {
				for _, repo := range configRepo.Repositories {
					if repo.Name == result.Name {
						cfg = repo.ConfigRepo
						exclude = *repo.Exclude
					}
				}
			}

			if exclude {
				continue
			}

			if gitlab.isReservedName(result.Name) {
				continue
			}

			fmt.Printf("\n%d. Evaluating GitHub repository '%s'\n", count, result.Name)
			prj := NewProject(gitlab, *configRepo.GitLabGroupID, source, configRepo.Username, result, cfg)
			if err := SyncRepo(prj); err != nil {
				fmt.Println(err)
			}

			count++
		}

		pageNum++
		results, err = source.Paginate(configRepo.Username, pageNum)
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
				//for _, asset := range release.Assets {
				//
				//}
			}
		}
	}

	return nil
}
