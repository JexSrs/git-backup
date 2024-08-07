package main

import (
	"fmt"
	"main/src/sources"
)

func SyncUser(gitlab *GitLab, groupId int, source sources.Source, username string) {
	pageNum := 0

	results, err := source.Paginate(username, pageNum)
	for true {
		if err != nil {
			fmt.Println(err)
			break
		}

		for _, result := range results {
			prj := NewProject(gitlab, groupId, source, username, result)
			if err := SyncRepo(prj); err != nil {
				fmt.Println(err)
			}
		}

		pageNum++
		results, err = source.Paginate(username, pageNum)
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

		fmt.Println("- Create 'original_url' attribute with value")
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
		fmt.Println(fmt.Sprintf("- Found %d protected branches", len(protectedBranches)))
		if err != nil {
			return err
		}

		fmt.Println("  - Unprotecting branches...")
		for _, branch := range protectedBranches {
			fmt.Println(fmt.Sprintf("    - Unprotecting %s...", branch))
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

		fmt.Println(fmt.Sprintf("  - Found %d branches", len(branches)))
		for _, branch := range branches {
			fmt.Println(fmt.Sprintf("  - Pushing %s...", branch))
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
	fmt.Println("- Checking for GitHub wiki...")
	prj.InitWikiProject()
	if err := prj.Wiki.CloneFromSource(); err == nil {
		fmt.Println("  - Found GitHub Wiki, syncing...")
		if err := prj.Wiki.AddRemoteToRepo(); err != nil {
			return err
		}

		fmt.Println("  - Pushing branches to GitLab...")
		branches, err := prj.Wiki.GetBranches()
		if err != nil {
			return err
		}

		fmt.Println(fmt.Sprintf("    - Found %d branches", len(branches)))
		for _, branch := range branches {
			fmt.Println(fmt.Sprintf("    - Pushing %s...", branch))
			if err := prj.Wiki.PushBranch(branch); err != nil {
				return err
			}
		}

		fmt.Println("  - Pushing tags to GitLab...")
		if err := prj.Wiki.PushAllTags(); err != nil {
			return err
		}
	}

	// Sync Releases
	
	return nil
}
