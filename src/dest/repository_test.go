package dest

import (
	"fmt"
	"main/src/configuration"
	"main/src/sources"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed; local clone/push tests need git-upload-pack and git-receive-pack")
	}
}

// newSourceRepo creates a repository with one commit on the default branch, a "feature" branch
// and a "v1.0.0" tag. It returns the directory, the repository, the commit hash and the default branch name.
func newSourceRepo(t *testing.T) (string, *git.Repository, plumbing.Hash, string) {
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
	if err := repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/feature", hash)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTag("v1.0.0", hash, nil); err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}

	return dir, repo, hash, head.Name().Short()
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

func withRemote(t *testing.T, repo *git.Repository, name, url string) {
	t.Helper()
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: name, URLs: []string{url}}); err != nil {
		t.Fatal(err)
	}
}

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// uniqueName returns a clone directory name that cannot collide with other tests or runs.
func uniqueName(t *testing.T) string {
	return fmt.Sprintf("git-backup-test-%d-%s", os.Getpid(), unsafeNameChars.ReplaceAllString(t.Name(), "-"))
}

func TestRepository_GetLocalBranches_ListsLocalBranchesWithoutRefPrefix(t *testing.T) {
	_, repo, _, defaultBranch := newSourceRepo(t)
	r := &Repository{LocalRepository: repo}

	got, err := r.GetLocalBranches()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(got)
	want := []string{"feature", defaultBranch}
	sort.Strings(want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("expected branches %v, got %v", want, got)
	}
}

func TestRepository_OperationsRequireALocalClone(t *testing.T) {
	r := &Repository{ID: "1"}

	if _, err := r.GetLocalBranches(); err == nil {
		t.Error("GetLocalBranches: expected an error without a clone")
	}
	if err := r.PushLocalBranch("main", "dst"); err == nil {
		t.Error("PushLocalBranch: expected an error without a clone")
	}
	if err := r.PushAllTags("dst"); err == nil {
		t.Error("PushAllTags: expected an error without a clone")
	}
}

func TestRepository_CloneFromSource_ClonesIntoTmpAndPruneRemovesIt(t *testing.T) {
	requireGit(t)
	srcDir, _, hash, _ := newSourceRepo(t)

	r := &Repository{Name: uniqueName(t), Remote: sources.SourceRepository{URL: srcDir}}
	t.Cleanup(func() { _ = r.Prune() })

	cfg := configuration.ConfigDestination{Timeout: time.Minute, IgnoreTLS: true}
	if err := r.CloneFromSource(&stubSource{}, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clonePath := filepath.Join("/tmp/git-backup", r.Name)
	if _, err := os.Stat(filepath.Join(clonePath, ".git")); err != nil {
		t.Fatalf("expected a clone at %s: %v", clonePath, err)
	}
	if r.LocalRepository == nil {
		t.Fatal("expected the local repository to be set")
	}
	head, err := r.LocalRepository.Head()
	if err != nil || head.Hash() != hash {
		t.Errorf("expected the clone to point at the source commit %s, got %v (%v)", hash, head, err)
	}
	if _, err := r.LocalRepository.Tag("v1.0.0"); err != nil {
		t.Errorf("expected tags to be cloned: %v", err)
	}
	if !r.DstIgnoreTLS {
		t.Errorf("expected the destination TLS settings to be remembered for later pushes")
	}

	if err := r.Prune(); err != nil {
		t.Fatalf("unexpected prune error: %v", err)
	}
	if _, err := os.Stat(clonePath); !os.IsNotExist(err) {
		t.Fatalf("expected the clone to be removed, stat returned %v", err)
	}
}

func TestRepository_CloneFromSource_FailsWhenTheSourceIsMissing(t *testing.T) {
	requireGit(t)

	r := &Repository{Name: uniqueName(t), Remote: sources.SourceRepository{URL: filepath.Join(t.TempDir(), "missing")}}
	t.Cleanup(func() { _ = r.Prune() })

	if err := r.CloneFromSource(&stubSource{}, configuration.ConfigDestination{Timeout: time.Minute}); err == nil {
		t.Fatal("expected an error for a missing source")
	}
}

func TestRepository_CloneFromSource_FailsWhenTheCABundleCannotBeRead(t *testing.T) {
	r := &Repository{Name: uniqueName(t), Remote: sources.SourceRepository{URL: t.TempDir()}}
	t.Cleanup(func() { _ = r.Prune() })

	cfg := configuration.ConfigDestination{Timeout: time.Minute, CACert: filepath.Join(t.TempDir(), "missing.pem")}
	if err := r.CloneFromSource(&stubSource{}, cfg); err == nil {
		t.Fatal("expected an error when the CA bundle cannot be read")
	}
}

func TestRepository_PushLocalBranch_PushesTheBranchToTheNamedRemote(t *testing.T) {
	requireGit(t)
	_, src, hash, _ := newSourceRepo(t)
	bareDir, bare := newBareRepo(t)
	withRemote(t, src, "dst", bareDir)

	r := &Repository{LocalRepository: src}
	if err := r.PushLocalBranch("feature", "dst"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ref, err := bare.Reference("refs/heads/feature", false)
	if err != nil {
		t.Fatalf("expected refs/heads/feature on the remote: %v", err)
	}
	if ref.Hash() != hash {
		t.Fatalf("expected the remote branch to point at %s, got %s", hash, ref.Hash())
	}
}

func TestRepository_PushLocalBranch_RenamesTheBranchWhenAnOverrideIsSet(t *testing.T) {
	requireGit(t)
	_, src, hash, _ := newSourceRepo(t)
	bareDir, bare := newBareRepo(t)
	withRemote(t, src, "dst", bareDir)

	r := &Repository{LocalRepository: src, OverrideRemoteBranch: "main"}
	if err := r.PushLocalBranch("feature", "dst"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ref, err := bare.Reference("refs/heads/main", false)
	if err != nil || ref.Hash() != hash {
		t.Fatalf("expected feature to be pushed as main, got (%v, %v)", ref, err)
	}
	if _, err := bare.Reference("refs/heads/feature", false); err == nil {
		t.Fatal("expected no refs/heads/feature on the remote when an override is set")
	}
}

func TestRepository_PushLocalBranch_AlreadyUpToDateIsNotAnError(t *testing.T) {
	requireGit(t)
	_, src, _, _ := newSourceRepo(t)
	bareDir, _ := newBareRepo(t)
	withRemote(t, src, "dst", bareDir)

	r := &Repository{LocalRepository: src}
	if err := r.PushLocalBranch("feature", "dst"); err != nil {
		t.Fatalf("unexpected error on first push: %v", err)
	}
	if err := r.PushLocalBranch("feature", "dst"); err != nil {
		t.Fatalf("expected a second identical push to succeed, got: %v", err)
	}
}

func TestRepository_PushLocalBranch_FailsForUnknownRemote(t *testing.T) {
	_, src, _, _ := newSourceRepo(t)

	r := &Repository{LocalRepository: src}
	if err := r.PushLocalBranch("feature", "nope"); err == nil {
		t.Fatal("expected an error for an unknown remote")
	}
}

func TestRepository_PushAllTags_PushesEveryTag(t *testing.T) {
	requireGit(t)
	_, src, hash, _ := newSourceRepo(t)
	bareDir, bare := newBareRepo(t)
	withRemote(t, src, "dst", bareDir)

	r := &Repository{LocalRepository: src}
	if err := r.PushAllTags("dst"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.PushAllTags("dst"); err != nil {
		t.Fatalf("expected a repeated tag push to succeed, got: %v", err)
	}

	ref, err := bare.Reference("refs/tags/v1.0.0", false)
	if err != nil {
		t.Fatalf("expected refs/tags/v1.0.0 on the remote: %v", err)
	}
	if ref.Hash() != hash {
		t.Fatalf("expected the tag to point at %s, got %s", hash, ref.Hash())
	}
}
