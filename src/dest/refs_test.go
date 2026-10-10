package dest

import (
	"fmt"
	"main/src/configuration"
	"main/src/sources"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// refsSource is the smallest sources.Source: it only has to answer FetchUsernamePassword.
type refsSource struct{}

func (refsSource) Paginate(string, configuration.ConfigGroup, *sources.PaginationResponse) (*sources.PaginationResponse, error) {
	return nil, nil
}
func (refsSource) GetWikiURL(string, string) string { return "" }
func (refsSource) FetchReleases(string, sources.SourceRepository) ([]sources.SourceRelease, error) {
	return nil, nil
}
func (refsSource) AddTokenToCloneUrl(u string) string      { return u }
func (refsSource) FetchUsernamePassword() (string, string) { return "", "" }

// h derives a distinct, valid hash from any short label.
func h(s string) plumbing.Hash { return plumbing.ComputeHash(plumbing.BlobObject, []byte(s)) }

// --- RefDifferences (pure) ---

func TestRefDifferences_IdenticalRefsHaveNoDifferences(t *testing.T) {
	refs := map[string]plumbing.Hash{"refs/heads/main": h("1"), "refs/tags/v1": h("2")}

	if got := RefDifferences(refs, refs); len(got) != 0 {
		t.Fatalf("expected no differences, got %v", got)
	}
}

func TestRefDifferences_ReportsRefsMissingFromTheDestination(t *testing.T) {
	src := map[string]plumbing.Hash{"refs/heads/main": h("1"), "refs/tags/v1": h("2")}
	dst := map[string]plumbing.Hash{"refs/heads/main": h("1")}

	got := RefDifferences(src, dst)
	if len(got) != 1 {
		t.Fatalf("expected 1 difference, got %v", got)
	}
	if got[0].Name != "refs/tags/v1" || got[0].Source != h("2") || !got[0].Destination.IsZero() {
		t.Fatalf("expected the missing tag with a zero destination hash, got %+v", got[0])
	}
}

func TestRefDifferences_ReportsRefsWithADifferentHash(t *testing.T) {
	src := map[string]plumbing.Hash{"refs/heads/main": h("1")}
	dst := map[string]plumbing.Hash{"refs/heads/main": h("9")}

	got := RefDifferences(src, dst)
	if len(got) != 1 || got[0].Source != h("1") || got[0].Destination != h("9") {
		t.Fatalf("expected one difference carrying both hashes, got %+v", got)
	}
}

func TestRefDifferences_IgnoresRefsOnlyPresentInTheDestination(t *testing.T) {
	src := map[string]plumbing.Hash{"refs/heads/main": h("1")}
	dst := map[string]plumbing.Hash{"refs/heads/main": h("1"), "refs/heads/old": h("5"), "refs/tags/v0": h("6")}

	if got := RefDifferences(src, dst); len(got) != 0 {
		t.Fatalf("expected extra destination refs to be ignored, got %v", got)
	}
}

func TestRefDifferences_AreSortedByName(t *testing.T) {
	src := map[string]plumbing.Hash{"refs/tags/v1": h("1"), "refs/heads/b": h("2"), "refs/heads/a": h("3")}

	got := RefDifferences(src, map[string]plumbing.Hash{})
	if len(got) != 3 || got[0].Name != "refs/heads/a" || got[1].Name != "refs/heads/b" || got[2].Name != "refs/tags/v1" {
		t.Fatalf("expected differences sorted by name, got %+v", got)
	}
}

func TestRefDifference_StringShowsBothHashesOrMissing(t *testing.T) {
	differing := RefDifference{Name: "refs/heads/main", Source: h("1"), Destination: h("9")}
	want := fmt.Sprintf("refs/heads/main: source %s, destination %s", h("1").String()[:7], h("9").String()[:7])
	if got := differing.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	missing := RefDifference{Name: "refs/tags/v1", Source: h("2")}
	want = fmt.Sprintf("refs/tags/v1: source %s, destination missing", h("2").String()[:7])
	if got := missing.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- ListRemoteRefs (local git) ---

func TestListRemoteRefs_ListsBranchesAndTagsWithoutCloning(t *testing.T) {
	requireGit(t)
	dir, _, hash, defaultBranch := newSourceRepo(t)

	refs, err := ListRemoteRefs(dir, nil, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]plumbing.Hash{
		"refs/heads/" + defaultBranch: hash,
		"refs/heads/feature":          hash,
		"refs/tags/v1.0.0":            hash,
	}
	if len(refs) != len(want) {
		t.Fatalf("expected exactly branches and tags %v, got %v", want, refs)
	}
	for name, h := range want {
		if refs[name] != h {
			t.Errorf("%s: expected %s, got %s", name, h, refs[name])
		}
	}
	if _, ok := refs["HEAD"]; ok {
		t.Errorf("expected HEAD not to be listed")
	}
}

func TestListRemoteRefs_EmptyRepositoryHasNoRefs(t *testing.T) {
	requireGit(t)
	dir, _ := newBareRepo(t)

	refs, err := ListRemoteRefs(dir, nil, false, nil)
	if err != nil {
		t.Fatalf("expected an empty repository to list without error, got %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no refs, got %v", refs)
	}
}

func TestListRemoteRefs_UnreachableRepositoryIsError(t *testing.T) {
	requireGit(t)

	if _, err := ListRemoteRefs(filepath.Join(t.TempDir(), "missing"), nil, false, nil); err == nil {
		t.Fatal("expected an error for a repository that does not exist")
	}
}

// --- Repository.IsUpToDate (local git) ---

// pushAllRefs mirrors every branch and tag of the repository at srcDir into the bare repository at dstDir.
func pushAllRefs(t *testing.T, srcDir, dstDir string) {
	t.Helper()
	repo, err := git.PlainOpen(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := repo.CreateRemote(&config.RemoteConfig{Name: "mirror", URLs: []string{dstDir}})
	if err != nil {
		t.Fatal(err)
	}
	err = remote.Push(&git.PushOptions{RemoteName: "mirror", RefSpecs: []config.RefSpec{
		"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*",
	}})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		t.Fatal(err)
	}
	if err := repo.DeleteRemote("mirror"); err != nil {
		t.Fatal(err)
	}
}

func addCommit(t *testing.T, dir string) plumbing.Hash {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CHANGES.md"), []byte("more"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("CHANGES.md"); err != nil {
		t.Fatal(err)
	}
	hash, err := wt.Commit("second", &git.CommitOptions{
		Author: &object.Signature{Name: "tester", Email: "tester@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func upToDateFixture(t *testing.T) (*Repository, string, string) {
	t.Helper()
	srcDir, _, _, defaultBranch := newSourceRepo(t)
	dstDir, _ := newBareRepo(t)
	r := &Repository{Name: uniqueName(t), Remote: sources.SourceRepository{Name: "alpha", URL: srcDir}}
	return r, dstDir, defaultBranch
}

func TestRepository_IsUpToDate_EmptyDestinationIsBehindOnEveryRef(t *testing.T) {
	requireGit(t)
	r, dstDir, _ := upToDateFixture(t)

	upToDate, diffs, err := r.IsUpToDate(refsSource{}, dstDir, configuration.ConfigDestination{Timeout: time.Minute})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if upToDate {
		t.Fatal("expected an empty destination to be behind")
	}
	if len(diffs) != 3 {
		t.Fatalf("expected 3 missing refs (2 branches, 1 tag), got %v", diffs)
	}
}

func TestRepository_IsUpToDate_MirroredDestinationIsUpToDate(t *testing.T) {
	requireGit(t)
	r, dstDir, _ := upToDateFixture(t)
	pushAllRefs(t, r.Remote.URL, dstDir)

	upToDate, diffs, err := r.IsUpToDate(refsSource{}, dstDir, configuration.ConfigDestination{Timeout: time.Minute})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !upToDate || len(diffs) != 0 {
		t.Fatalf("expected a mirrored destination to be up to date, got upToDate=%v diffs=%v", upToDate, diffs)
	}
}

func TestRepository_IsUpToDate_NewSourceCommitMakesTheDestinationBehind(t *testing.T) {
	requireGit(t)
	r, dstDir, defaultBranch := upToDateFixture(t)
	pushAllRefs(t, r.Remote.URL, dstDir)
	newHash := addCommit(t, r.Remote.URL)

	upToDate, diffs, err := r.IsUpToDate(refsSource{}, dstDir, configuration.ConfigDestination{Timeout: time.Minute})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if upToDate {
		t.Fatal("expected a new commit on the source to leave the destination behind")
	}
	if len(diffs) != 1 || diffs[0].Name != "refs/heads/"+defaultBranch || diffs[0].Source != newHash || diffs[0].Destination.IsZero() {
		t.Fatalf("expected exactly the default branch to differ with both hashes, got %+v", diffs)
	}
}

func TestRepository_IsUpToDate_WithoutADestinationURLIsError(t *testing.T) {
	requireGit(t)
	r, _, _ := upToDateFixture(t)

	if _, _, err := r.IsUpToDate(refsSource{}, "", configuration.ConfigDestination{Timeout: time.Minute}); err == nil {
		t.Fatal("expected an error without a destination url")
	}
}

func TestRepository_IsUpToDate_UnreachableSourceIsError(t *testing.T) {
	requireGit(t)
	r, dstDir, _ := upToDateFixture(t)
	r.Remote.URL = filepath.Join(t.TempDir(), "missing")

	if _, _, err := r.IsUpToDate(refsSource{}, dstDir, configuration.ConfigDestination{Timeout: time.Minute}); err == nil {
		t.Fatal("expected an error when the source cannot be listed")
	}
}
