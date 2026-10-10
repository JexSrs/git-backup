package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed; local clone/push tests need git-upload-pack and git-receive-pack")
	}
}

// newSourceRepo creates a repository with one commit on the default branch and a "v1.0.0" tag.
// It returns the directory, the commit hash and the default branch name.
func newSourceRepo(t *testing.T) (string, plumbing.Hash, string) {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatal(err)
	}
	hash, err := wt.Commit("initial", &git.CommitOptions{
		Author: &object.Signature{Name: "tester", Email: "tester@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTag("v1.0.0", hash, nil); err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}

	return dir, hash, head.Name().Short()
}

func newBareRepo(t *testing.T) (string, *git.Repository) {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	return dir, repo
}

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// uniqueName returns a clone directory name that cannot collide with other tests or runs.
func uniqueName(t *testing.T) string {
	return fmt.Sprintf("git-backup-test-%d-%s", os.Getpid(), unsafeNameChars.ReplaceAllString(t.Name(), "-"))
}

func clonePath(name string) string {
	return filepath.Join("/tmp/git-backup", name)
}
