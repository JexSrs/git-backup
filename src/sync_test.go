package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"main/src/configuration"
	"main/src/dest"
	"main/src/sources"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ico "github.com/biessek/golang-ico"
	"github.com/go-git/go-git/v5/plumbing"
)

type syncFixture struct {
	git     *fakeDest
	storage *fakeDest
	dsts    map[string]dest.Destination
	source  *fakeSource
}

func newSyncFixture() *syncFixture {
	git := newFakeDest("gitea")
	storage := newFakeDest("dufs")
	return &syncFixture{
		git:     git,
		storage: storage,
		dsts:    map[string]dest.Destination{"gitea": git, "dufs": storage},
		source:  &fakeSource{},
	}
}

// fileServer serves the given files by path and counts the requests it receives.
func fileServer(t *testing.T, files map[string][]byte) (*httptest.Server, *int) {
	t.Helper()

	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		content, ok := files[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(content)
	}))
	t.Cleanup(server.Close)

	return server, &hits
}

func expectCalls(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected destination calls:\n got: %v\nwant: %v", got, want)
	}
}

// --- import path ---

func TestSyncRepo_ImportsAMissingRepositoryAndUnprotectsItsBranches(t *testing.T) {
	f := newSyncFixture()
	f.git.protected = []string{"main", "release"}

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), repoConfig("gitea", "dufs"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls, []string{
		"EnsureOwner:octo",
		"RetrieveExistingRepo:alpha",
		"ImportRepository:alpha",
		"LockUntilImport:alpha",
		"SetOriginalUrl:https://github.com/octo/alpha.git",
		"GetProtectedBranches",
		"UnprotectBranch:main",
		"UnprotectBranch:release",
	})
	if len(f.storage.calls) != 0 {
		t.Fatalf("expected no storage calls, got %v", f.storage.calls)
	}
}

func TestSyncRepo_DoesNotWaitForTheImportOfAnEmptyRepository(t *testing.T) {
	f := newSyncFixture()
	remote := remoteNamed("alpha")
	remote.Empty = true

	if err := SyncRepo(f.dsts, f.source, remote, repoConfig("gitea", "dufs"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls, []string{
		"EnsureOwner:octo",
		"RetrieveExistingRepo:alpha",
		"ImportRepository:alpha",
		"SetOriginalUrl:https://github.com/octo/alpha.git",
		"GetProtectedBranches",
	})
}

func TestSyncRepo_ResumesAnUnfinishedMigration(t *testing.T) {
	f := newSyncFixture()
	remote := remoteNamed("alpha")
	f.git.existing["alpha"] = &dest.Repository{ID: "7", Name: "alpha", FinishedMiration: false, Remote: remote}

	if err := SyncRepo(f.dsts, f.source, remote, repoConfig("gitea", "dufs"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls, []string{
		"EnsureOwner:octo",
		"RetrieveExistingRepo:alpha",
		"LockUntilImport:alpha",
		"SetOriginalUrl:https://github.com/octo/alpha.git",
		"GetProtectedBranches",
	})
}

func TestSyncRepo_SkipsAnExistingRepositoryWhenOnlyNewIsSet(t *testing.T) {
	f := newSyncFixture()
	remote := remoteNamed("alpha")
	f.git.existing["alpha"] = &dest.Repository{ID: "7", Name: "alpha", FinishedMiration: true, Remote: remote}
	group := groupConfig()
	group.Filter.OnlyNew = ptr(true)

	if err := SyncRepo(f.dsts, f.source, remote, repoConfig("gitea", "dufs"), group); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls, []string{"EnsureOwner:octo", "RetrieveExistingRepo:alpha"})
}

func TestSyncRepo_ArchivesTheDestinationWhenTheSourceIsArchived(t *testing.T) {
	f := newSyncFixture()
	remote := remoteNamed("alpha")
	remote.Archived = true

	if err := SyncRepo(f.dsts, f.source, remote, repoConfig("gitea", "dufs"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if last := f.git.calls[len(f.git.calls)-1]; last != "ChangeArchivedState:true" {
		t.Fatalf("expected the repository to be archived last, got calls %v", f.git.calls)
	}
}

func TestSyncRepo_WrapsDestinationFailures(t *testing.T) {
	tests := []struct {
		method  string
		wantErr string
	}{
		{"EnsureOwner", "failed to ensure repository owner exists"},
		{"RetrieveExistingRepo", "failed to retrieve existing repo"},
		{"ImportRepository", "failed to import project"},
		{"LockUntilImport", "failed to read import status"},
		{"SetOriginalUrl", "failed to set original url"},
		{"GetProtectedBranches", "failed to get list of protected branches"},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			f := newSyncFixture()
			f.git.errs[tc.method] = errors.New("boom")

			err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), repoConfig("gitea", "dufs"), groupConfig())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "boom") {
				t.Fatalf("expected %q wrapping boom, got %q", tc.wantErr, err)
			}
		})
	}
}

// --- avatar ---

func TestSyncRepo_UploadsTheSourceAvatar(t *testing.T) {
	f := newSyncFixture()
	server, _ := fileServer(t, map[string][]byte{"/avatar.png": []byte("PNGDATA")})
	remote := remoteNamed("alpha")
	remote.Avatar = ptr(server.URL + "/avatar.png")
	cfg := repoConfig("gitea", "dufs")
	cfg.FetchAvatar = ptr(true)

	if err := SyncRepo(f.dsts, f.source, remote, cfg, groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := f.git.callsWithPrefix("ChangeAvatar:"); !reflect.DeepEqual(got, []string{"png"}) {
		t.Fatalf("expected one png avatar upload, got %v", got)
	}
	if string(f.git.avatars["png"]) != "PNGDATA" {
		t.Fatalf("expected the downloaded bytes to be uploaded, got %q", f.git.avatars["png"])
	}
}

func TestSyncRepo_ConvertsIcoAvatarsToPng(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for x := 0; x < 2; x++ {
		for y := 0; y < 2; y++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	icon := &bytes.Buffer{}
	if err := ico.Encode(icon, img); err != nil {
		t.Fatal(err)
	}

	f := newSyncFixture()
	server, _ := fileServer(t, map[string][]byte{"/favicon.ico": icon.Bytes()})
	remote := remoteNamed("alpha")
	remote.Avatar = ptr(server.URL + "/favicon.ico")
	cfg := repoConfig("gitea", "dufs")
	cfg.FetchAvatar = ptr(true)

	if err := SyncRepo(f.dsts, f.source, remote, cfg, groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := f.git.callsWithPrefix("ChangeAvatar:"); !reflect.DeepEqual(got, []string{"png"}) {
		t.Fatalf("expected the icon to be uploaded as png, got %v", got)
	}
	decoded, err := png.Decode(bytes.NewReader(f.git.avatars["png"]))
	if err != nil {
		t.Fatalf("expected a valid PNG avatar: %v", err)
	}
	if decoded.Bounds().Dx() != 2 {
		t.Fatalf("expected the converted image to keep its size, got %v", decoded.Bounds())
	}
}

func TestSyncRepo_SkipsTheAvatarWhenDisabledOrUnsupported(t *testing.T) {
	t.Run("fetch_avatar false", func(t *testing.T) {
		f := newSyncFixture()
		server, hits := fileServer(t, map[string][]byte{"/avatar.png": []byte("PNGDATA")})
		remote := remoteNamed("alpha")
		remote.Avatar = ptr(server.URL + "/avatar.png")

		if err := SyncRepo(f.dsts, f.source, remote, repoConfig("gitea", "dufs"), groupConfig()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if *hits != 0 || len(f.git.callsWithPrefix("ChangeAvatar:")) != 0 {
			t.Fatalf("expected no avatar download or upload, got %d hits and calls %v", *hits, f.git.calls)
		}
	})

	t.Run("destination without avatar support", func(t *testing.T) {
		f := newSyncFixture()
		f.git.caps.Avatar = false
		server, hits := fileServer(t, map[string][]byte{"/avatar.png": []byte("PNGDATA")})
		remote := remoteNamed("alpha")
		remote.Avatar = ptr(server.URL + "/avatar.png")
		cfg := repoConfig("gitea", "dufs")
		cfg.FetchAvatar = ptr(true)

		if err := SyncRepo(f.dsts, f.source, remote, cfg, groupConfig()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if *hits != 0 || len(f.git.callsWithPrefix("ChangeAvatar:")) != 0 {
			t.Fatalf("expected no avatar download or upload, got %d hits and calls %v", *hits, f.git.calls)
		}
	})
}

func TestSyncRepo_AvatarDownloadFailureIsAnError(t *testing.T) {
	f := newSyncFixture()
	server, _ := fileServer(t, map[string][]byte{})
	remote := remoteNamed("alpha")
	remote.Avatar = ptr(server.URL + "/missing.png")
	cfg := repoConfig("gitea", "dufs")
	cfg.FetchAvatar = ptr(true)

	err := SyncRepo(f.dsts, f.source, remote, cfg, groupConfig())
	if err == nil || !strings.Contains(err.Error(), "failed to download avatar") {
		t.Fatalf("expected an avatar download error, got %v", err)
	}
}

// --- releases ---

func releaseConfig(maxSize string) configuration.ConfigRepo {
	cfg := repoConfig("gitea", "dufs")
	cfg.Releases.Exclude = ptr(false)
	cfg.Releases.Assets.MaxSize = ptr(maxSize)
	return cfg
}

func TestSyncRepo_CreatesMissingReleasesAndStoresTheirAssets(t *testing.T) {
	f := newSyncFixture()
	server, _ := fileServer(t, map[string][]byte{"/app.zip": []byte("ZIPDATA")})
	f.source.releases = map[string][]sources.SourceRelease{
		"alpha": {{Name: "v1", TagName: "v1.0.0", Assets: []sources.SourceAsset{{Name: "app.zip", URL: server.URL + "/app.zip"}}}},
	}

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), releaseConfig("1GB"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls[6:], []string{
		"ReleaseExists:v1.0.0",
		"CreateRelease:v1.0.0",
		"LinkAsset:v1.0.0:app.zip:https://dufs.local/stored/gitea/repositories/repo_id-alpha/tag_v1.0.0/app.zip",
	})
	expectCalls(t, f.storage.calls, []string{"UploadFile:/gitea/repositories/repo_id-alpha/tag_v1.0.0/app.zip"})
	if got := f.storage.uploads["/gitea/repositories/repo_id-alpha/tag_v1.0.0/app.zip"]; string(got) != "ZIPDATA" {
		t.Fatalf("expected the downloaded asset to be stored, got %q", got)
	}
}

func TestSyncRepo_SkipsReleasesThatAlreadyExist(t *testing.T) {
	f := newSyncFixture()
	server, hits := fileServer(t, map[string][]byte{"/app.zip": []byte("ZIPDATA")})
	f.source.releases = map[string][]sources.SourceRelease{
		"alpha": {{Name: "v1", TagName: "v1.0.0", Assets: []sources.SourceAsset{{Name: "app.zip", URL: server.URL + "/app.zip"}}}},
	}
	f.git.releases["v1.0.0"] = true

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), releaseConfig("1GB"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls[6:], []string{"ReleaseExists:v1.0.0"})
	if *hits != 0 || len(f.storage.calls) != 0 {
		t.Fatalf("expected nothing to be downloaded or stored for an existing release, got %d hits and %v", *hits, f.storage.calls)
	}
}

func TestSyncRepo_LinksOversizedAssetsToTheirOriginalURL(t *testing.T) {
	f := newSyncFixture()
	server, _ := fileServer(t, map[string][]byte{"/big.bin": bytes.Repeat([]byte("x"), 3000)})
	f.source.releases = map[string][]sources.SourceRelease{
		"alpha": {{Name: "v1", TagName: "v1.0.0", Assets: []sources.SourceAsset{{Name: "big.bin", URL: server.URL + "/big.bin"}}}},
	}

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), releaseConfig("2KB"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(f.storage.calls) != 0 {
		t.Fatalf("expected an oversized asset not to be stored, got %v", f.storage.calls)
	}
	if got := f.git.callsWithPrefix("LinkAsset:"); !reflect.DeepEqual(got, []string{"v1.0.0:big.bin:" + server.URL + "/big.bin"}) {
		t.Fatalf("expected the original url to be linked, got %v", got)
	}
}

func TestSyncRepo_MaxSizeNoneStoresEveryAsset(t *testing.T) {
	f := newSyncFixture()
	server, _ := fileServer(t, map[string][]byte{"/big.bin": bytes.Repeat([]byte("x"), 3000)})
	f.source.releases = map[string][]sources.SourceRelease{
		"alpha": {{Name: "v1", TagName: "v1.0.0", Assets: []sources.SourceAsset{{Name: "big.bin", URL: server.URL + "/big.bin"}}}},
	}

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), releaseConfig("none"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.storage.calls, []string{"UploadFile:/gitea/repositories/repo_id-alpha/tag_v1.0.0/big.bin"})
}

func TestSyncRepo_LinksTheOriginalAssetURLWhenAssetSyncIsExcluded(t *testing.T) {
	f := newSyncFixture()
	server, hits := fileServer(t, map[string][]byte{"/app.zip": []byte("ZIPDATA")})
	f.source.releases = map[string][]sources.SourceRelease{
		"alpha": {{Name: "v1", TagName: "v1.0.0", Assets: []sources.SourceAsset{{Name: "app.zip", URL: server.URL + "/app.zip"}}}},
	}
	cfg := releaseConfig("1GB")
	cfg.Releases.Assets.Exclude = ptr(true)

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), cfg, groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *hits != 0 || len(f.storage.calls) != 0 {
		t.Fatalf("expected no download or upload, got %d hits and %v", *hits, f.storage.calls)
	}
	if got := f.git.callsWithPrefix("LinkAsset:"); !reflect.DeepEqual(got, []string{"v1.0.0:app.zip:" + server.URL + "/app.zip"}) {
		t.Fatalf("expected the original url to be linked, got %v", got)
	}
}

func TestSyncRepo_SanitizesTagAndAssetNamesInStoragePaths(t *testing.T) {
	f := newSyncFixture()
	server, _ := fileServer(t, map[string][]byte{"/file.bin": []byte("DATA")})
	f.source.releases = map[string][]sources.SourceRelease{
		"alpha": {{Name: "r", TagName: "release/1", Assets: []sources.SourceAsset{{Name: "dir/file.bin", URL: server.URL + "/file.bin"}}}},
	}

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), releaseConfig("1GB"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.storage.calls, []string{"UploadFile:/gitea/repositories/repo_id-alpha/tag_release-1/dir-file.bin"})
}

func TestSyncRepo_DefaultConfigurationStoresTypicalAssets(t *testing.T) {
	var c configuration.Configuration
	c.PopulateDefault()
	cfg := c.Config
	cfg.Destination = ptr("gitea")
	cfg.Releases.Assets.Destination = ptr("dufs")
	cfg.FetchAvatar = ptr(false)
	cfg.Wiki.Exclude = ptr(true)

	f := newSyncFixture()
	server, _ := fileServer(t, map[string][]byte{"/app.zip": []byte("ZIPDATA")})
	f.source.releases = map[string][]sources.SourceRelease{
		"alpha": {{Name: "v1", TagName: "v1.0.0", Assets: []sources.SourceAsset{{Name: "app.zip", URL: server.URL + "/app.zip"}}}},
	}

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), cfg, groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.storage.calls, []string{"UploadFile:/gitea/repositories/repo_id-alpha/tag_v1.0.0/app.zip"})
}

func TestSyncRepo_SkipsReleasesWhenExcludedOrUnsupported(t *testing.T) {
	t.Run("releases excluded", func(t *testing.T) {
		f := newSyncFixture()
		f.source.releases = map[string][]sources.SourceRelease{"alpha": {{TagName: "v1.0.0"}}}

		if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), repoConfig("gitea", "dufs"), groupConfig()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.git.callsWithPrefix("ReleaseExists:")) != 0 {
			t.Fatalf("expected no release handling, got %v", f.git.calls)
		}
	})

	t.Run("destination without release support", func(t *testing.T) {
		f := newSyncFixture()
		f.git.caps.Releases = false
		f.source.releases = map[string][]sources.SourceRelease{"alpha": {{TagName: "v1.0.0"}}}

		if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), releaseConfig("1GB"), groupConfig()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.git.callsWithPrefix("ReleaseExists:")) != 0 {
			t.Fatalf("expected no release handling, got %v", f.git.calls)
		}
	})
}

func TestSyncRepo_ReleaseFailuresAreErrors(t *testing.T) {
	t.Run("fetching releases", func(t *testing.T) {
		f := newSyncFixture()
		f.source.releasesErr = errors.New("rate limited")

		err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), releaseConfig("1GB"), groupConfig())
		if err == nil || !strings.Contains(err.Error(), "releases: failed to fetch") {
			t.Fatalf("expected a fetch error, got %v", err)
		}
	})

	t.Run("downloading an asset", func(t *testing.T) {
		f := newSyncFixture()
		server, _ := fileServer(t, map[string][]byte{})
		f.source.releases = map[string][]sources.SourceRelease{
			"alpha": {{TagName: "v1.0.0", Assets: []sources.SourceAsset{{Name: "app.zip", URL: server.URL + "/app.zip"}}}},
		}

		err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), releaseConfig("1GB"), groupConfig())
		if err == nil || !strings.Contains(err.Error(), "asset: failed to download") {
			t.Fatalf("expected a download error, got %v", err)
		}
	})

	t.Run("storing an asset", func(t *testing.T) {
		f := newSyncFixture()
		f.storage.errs["UploadFile"] = errors.New("disk full")
		server, _ := fileServer(t, map[string][]byte{"/app.zip": []byte("ZIPDATA")})
		f.source.releases = map[string][]sources.SourceRelease{
			"alpha": {{TagName: "v1.0.0", Assets: []sources.SourceAsset{{Name: "app.zip", URL: server.URL + "/app.zip"}}}},
		}

		err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), releaseConfig("1GB"), groupConfig())
		if err == nil || !strings.Contains(err.Error(), "asset: failed to upload") {
			t.Fatalf("expected an upload error, got %v", err)
		}
	})
}

// --- wiki ---

func wikiConfig() configuration.ConfigRepo {
	cfg := repoConfig("gitea", "dufs")
	cfg.Wiki.Exclude = ptr(false)
	return cfg
}

func TestSyncRepo_SkipsTheWikiWhenTheSourceHasNone(t *testing.T) {
	f := newSyncFixture()

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), wikiConfig(), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if last := f.git.calls[len(f.git.calls)-1]; last != "GetWikiProject:alpha" {
		t.Fatalf("expected the wiki to be looked up and nothing more, got %v", f.git.calls)
	}
}

func TestSyncRepo_IgnoresAWikiThatCannotBeCloned(t *testing.T) {
	requireGit(t)
	f := newSyncFixture()
	wiki := &dest.Repository{Name: uniqueName(t) + ".wiki", Remote: sources.SourceRepository{URL: filepath.Join(t.TempDir(), "missing")}}
	f.git.wikis["alpha"] = wiki
	t.Cleanup(func() { _ = wiki.Prune() })

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), wikiConfig(), groupConfig()); err != nil {
		t.Fatalf("expected a missing wiki to be non-fatal, got %v", err)
	}

	if len(f.git.callsWithPrefix("CreateWiki:")) != 0 {
		t.Fatalf("expected no wiki to be created, got %v", f.git.calls)
	}
}

func TestSyncRepo_SyncsTheWikiToTheDestination(t *testing.T) {
	requireGit(t)
	wikiSrc, hash, _ := newSourceRepo(t)
	bareDir, bare := newBareRepo(t)

	f := newSyncFixture()
	wiki := &dest.Repository{Name: uniqueName(t) + ".wiki", Remote: sources.SourceRepository{URL: wikiSrc}, OverrideRemoteBranch: "main"}
	f.git.wikis["alpha"] = wiki
	f.git.remotes[wiki.Name] = bareDir

	if err := SyncRepo(f.dsts, f.source, remoteNamed("alpha"), wikiConfig(), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls[6:], []string{"GetWikiProject:alpha", "AddRemoteToRepo:" + wiki.Name, "CreateWiki:alpha"})
	ref, err := bare.Reference("refs/heads/main", false)
	if err != nil || ref.Hash() != hash {
		t.Fatalf("expected the wiki to be pushed to main, got (%v, %v)", ref, err)
	}
	if _, err := bare.Reference("refs/tags/v1.0.0", false); err != nil {
		t.Fatalf("expected wiki tags to be pushed: %v", err)
	}
	if _, err := os.Stat(clonePath(wiki.Name)); !os.IsNotExist(err) {
		t.Fatalf("expected the wiki clone to be pruned, stat returned %v", err)
	}
}

// --- existing repository ---

func TestSyncRepo_PushesAnExistingRepositoryToTheDestination(t *testing.T) {
	requireGit(t)
	srcDir, hash, defaultBranch := newSourceRepo(t)
	bareDir, bare := newBareRepo(t)

	f := newSyncFixture()
	name := uniqueName(t)
	remote := remoteNamed("alpha")
	remote.URL = srcDir
	f.git.existing["alpha"] = &dest.Repository{ID: "7", Name: name, FinishedMiration: true, Remote: remote}
	f.git.remotes[name] = bareDir

	if err := SyncRepo(f.dsts, f.source, remote, repoConfig("gitea", "dufs"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls, []string{
		"EnsureOwner:octo",
		"RetrieveExistingRepo:alpha",
		"ChangeArchivedState:false",
		"AddRemoteToRepo:" + name,
	})
	ref, err := bare.Reference(plumbing.ReferenceName("refs/heads/"+defaultBranch), false)
	if err != nil || ref.Hash() != hash {
		t.Fatalf("expected the default branch to be pushed, got (%v, %v)", ref, err)
	}
	tag, err := bare.Reference("refs/tags/v1.0.0", false)
	if err != nil || tag.Hash() != hash {
		t.Fatalf("expected the tag to be pushed, got (%v, %v)", tag, err)
	}
	if _, err := os.Stat(clonePath(name)); !os.IsNotExist(err) {
		t.Fatalf("expected the clone to be pruned, stat returned %v", err)
	}
}

func TestSyncRepo_SkipsTheCloneWhenTheDestinationAlreadyHasEveryBranchAndTag(t *testing.T) {
	requireGit(t)
	srcDir, _, _ := newSourceRepo(t)
	bareDir, _ := newBareRepo(t)
	pushAllRefs(t, srcDir, bareDir)

	f := newSyncFixture()
	name := uniqueName(t)
	remote := remoteNamed("alpha")
	remote.URL = srcDir
	f.git.existing["alpha"] = &dest.Repository{ID: "7", Name: name, FinishedMiration: true, Remote: remote}
	f.git.remotes[name] = bareDir

	if err := SyncRepo(f.dsts, f.source, remote, repoConfig("gitea", "dufs"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls, []string{
		"EnsureOwner:octo",
		"RetrieveExistingRepo:alpha",
		"ChangeArchivedState:false",
	})
	if _, err := os.Stat(clonePath(name)); !os.IsNotExist(err) {
		t.Fatalf("expected no clone to be made, stat returned %v", err)
	}
}

func TestSyncRepo_SyncsNormallyWhenTheUpdateCheckCannotRun(t *testing.T) {
	requireGit(t)
	srcDir, hash, defaultBranch := newSourceRepo(t)
	bareDir, bare := newBareRepo(t)

	f := newSyncFixture()
	name := uniqueName(t)
	remote := remoteNamed("alpha")
	remote.URL = srcDir
	f.git.existing["alpha"] = &dest.Repository{ID: "7", Name: name, FinishedMiration: true, Remote: remote}
	f.git.remotes[name] = bareDir
	f.git.remoteURLs[name] = filepath.Join(t.TempDir(), "unreachable") // the comparison fails, the push must still happen

	if err := SyncRepo(f.dsts, f.source, remote, repoConfig("gitea", "dufs"), groupConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectCalls(t, f.git.calls, []string{
		"EnsureOwner:octo",
		"RetrieveExistingRepo:alpha",
		"ChangeArchivedState:false",
		"AddRemoteToRepo:" + name,
	})
	ref, err := bare.Reference(plumbing.ReferenceName("refs/heads/"+defaultBranch), false)
	if err != nil || ref.Hash() != hash {
		t.Fatalf("expected the default branch to be pushed despite the failed check, got (%v, %v)", ref, err)
	}
}

func TestSyncRepo_ExistingRepositoryCloneFailureIsAnError(t *testing.T) {
	requireGit(t)
	f := newSyncFixture()
	name := uniqueName(t)
	remote := remoteNamed("alpha")
	remote.URL = filepath.Join(t.TempDir(), "missing")
	f.git.existing["alpha"] = &dest.Repository{ID: "7", Name: name, FinishedMiration: true, Remote: remote}
	t.Cleanup(func() { _ = os.RemoveAll(clonePath(name)) })

	err := SyncRepo(f.dsts, f.source, remote, repoConfig("gitea", "dufs"), groupConfig())
	if err == nil || !strings.Contains(err.Error(), "failed to clone source") {
		t.Fatalf("expected a clone error, got %v", err)
	}
}

// --- SyncUser ---

func pagesOf(names ...[]string) [][]sources.SourceRepository {
	pages := make([][]sources.SourceRepository, 0, len(names))
	for _, page := range names {
		repos := make([]sources.SourceRepository, 0, len(page))
		for _, name := range page {
			repos = append(repos, remoteNamed(name))
		}
		pages = append(pages, repos)
	}
	return pages
}

func TestSyncUser_SyncsEveryRepositoryAcrossPages(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha", "beta"}, []string{"gamma"})

	SyncUser(f.dsts, groupConfig(), f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("expected every repository to be imported in order, got %v", got)
	}
}

func TestSyncUser_SkipsTheFirstRepositories(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha", "beta"}, []string{"gamma"})
	group := groupConfig()
	group.Skip = ptr(2)

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"gamma"}) {
		t.Fatalf("expected only gamma to be imported, got %v", got)
	}
}

func TestSyncUser_IncludeOnlyIsCaseInsensitive(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha", "beta", "gamma"})
	group := groupConfig()
	group.IncludeOnly = []string{"BETA"}

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"beta"}) {
		t.Fatalf("expected only beta to be imported, got %v", got)
	}
}

func TestSyncUser_ExcludeIsCaseInsensitive(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha", "beta", "gamma"})
	group := groupConfig()
	group.Exclude = []string{"Alpha"}

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"beta", "gamma"}) {
		t.Fatalf("expected alpha to be excluded, got %v", got)
	}
}

func TestSyncUser_AppliesTheGroupFilter(t *testing.T) {
	f := newSyncFixture()
	f.source.pages = pagesOf([]string{"alpha", "beta", "gamma"})
	f.source.pages[0][1].Archived = true
	group := groupConfig()
	group.Filter.Archived = ptr(false)

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"alpha", "gamma"}) {
		t.Fatalf("expected the archived repository to be filtered out, got %v", got)
	}
}

func TestSyncUser_StopsWhenPaginationFails(t *testing.T) {
	t.Run("on a later page", func(t *testing.T) {
		f := newSyncFixture()
		f.source.pages = pagesOf([]string{"alpha", "beta"}, []string{"gamma"})
		f.source.failOnPage = 2

		SyncUser(f.dsts, groupConfig(), f.source)

		if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
			t.Fatalf("expected the first page to be synced before stopping, got %v", got)
		}
	})

	t.Run("on the first page", func(t *testing.T) {
		f := newSyncFixture()
		f.source.pages = pagesOf([]string{"alpha"})
		f.source.failOnPage = 1

		SyncUser(f.dsts, groupConfig(), f.source)

		if len(f.git.calls) != 0 {
			t.Fatalf("expected nothing to be synced, got %v", f.git.calls)
		}
	})
}

func TestSyncUser_UsesRepositorySpecificConfiguration(t *testing.T) {
	f := newSyncFixture()
	other := newFakeDest("gitea2")
	f.dsts["gitea2"] = other
	f.source.pages = pagesOf([]string{"alpha", "beta", "gamma"})
	group := groupConfig()
	group.Repositories = []configuration.ConfigRepository{
		{Name: "Beta", ConfigRepo: repoConfig("gitea2", "dufs")},
	}

	SyncUser(f.dsts, group, f.source)

	if got := f.git.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"alpha", "gamma"}) {
		t.Fatalf("expected alpha and gamma on the group destination, got %v", got)
	}
	if got := other.callsWithPrefix("ImportRepository:"); !reflect.DeepEqual(got, []string{"beta"}) {
		t.Fatalf("expected beta on its own destination, got %v", got)
	}
}

// --- skipFromFilter ---

func TestSkipFromFilter(t *testing.T) {
	private := sources.SourceRepository{Private: true}
	public := sources.SourceRepository{Private: false}
	archived := sources.SourceRepository{Archived: true}
	empty := sources.SourceRepository{Empty: true}
	described := sources.SourceRepository{Description: ptr("a repo")}
	blankDescription := sources.SourceRepository{Description: ptr("")}
	forked := sources.SourceRepository{Forked: true}

	tests := []struct {
		name   string
		remote sources.SourceRepository
		filter configuration.ConfigFilter
		skip   bool
	}{
		{"no filter keeps everything", private, configuration.ConfigFilter{}, false},
		{"public-only skips private", private, configuration.ConfigFilter{Visibility: []string{"public"}}, true},
		{"public-only keeps public", public, configuration.ConfigFilter{Visibility: []string{"public"}}, false},
		{"private-only skips public", public, configuration.ConfigFilter{Visibility: []string{"private"}}, true},
		{"visibility is case-insensitive", private, configuration.ConfigFilter{Visibility: []string{"PRIVATE"}}, false},
		{"both visibilities keep private", private, configuration.ConfigFilter{Visibility: []string{"public", "private"}}, false},
		{"archived=false skips archived", archived, configuration.ConfigFilter{Archived: ptr(false)}, true},
		{"archived=true keeps archived", archived, configuration.ConfigFilter{Archived: ptr(true)}, false},
		{"archived=true skips active", public, configuration.ConfigFilter{Archived: ptr(true)}, true},
		{"empty=false skips empty", empty, configuration.ConfigFilter{Empty: ptr(false)}, true},
		{"empty=true keeps empty", empty, configuration.ConfigFilter{Empty: ptr(true)}, false},
		{"has_description=true skips missing description", public, configuration.ConfigFilter{HasDescription: ptr(true)}, true},
		{"has_description=true skips blank description", blankDescription, configuration.ConfigFilter{HasDescription: ptr(true)}, true},
		{"has_description=true keeps described", described, configuration.ConfigFilter{HasDescription: ptr(true)}, false},
		{"has_description=false skips described", described, configuration.ConfigFilter{HasDescription: ptr(false)}, true},
		{"forked=false skips forks", forked, configuration.ConfigFilter{Forked: ptr(false)}, true},
		{"forked=true keeps forks", forked, configuration.ConfigFilter{Forked: ptr(true)}, false},
		{"forked=true skips originals", public, configuration.ConfigFilter{Forked: ptr(true)}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := skipFromFilter(tc.remote, tc.filter); got != tc.skip {
				t.Fatalf("skipFromFilter = %v, want %v", got, tc.skip)
			}
		})
	}
}
