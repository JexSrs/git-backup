package configuration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"main/src/utils"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// writeCAPEM writes a self-signed CA certificate to a temp file and returns its path and the parsed cert.
func writeCAPEM(t *testing.T) (string, *x509.Certificate) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "git-backup test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	return path, cert
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func intPtr(i int) *int       { return &i }

// validConfig returns a configuration that passes Validate after PopulateDefault.
func validConfig() Configuration {
	return Configuration{
		Config: ConfigRepo{
			Destination: strPtr("gitlab"),
			Releases: ConfigRepoReleases{
				Assets: ConfigRepoAssets{Destination: strPtr("dufs")},
			},
		},
		Destinations: []ConfigDestination{
			{ID: "gitlab", URL: "https://gitlab.example.com", Token: "t"},
			{ID: "dufs", URL: "https://dufs.example.com"},
		},
		Sources: []ConfigSource{
			{ID: "github", Auth: ConfigSourceAuth{Token: "gh"}},
			{ID: "git-proxmox"},
		},
		Groups: []ConfigGroup{
			{
				Source:   "github",
				Username: "octo",
				Repositories: []ConfigRepository{
					{Name: "repo"},
				},
			},
		},
	}
}

// --- PopulateDefault ---

func TestPopulateDefault_GlobalDefaults(t *testing.T) {
	c := Configuration{}
	c.PopulateDefault()

	if got := c.Filter.Visibility; !reflect.DeepEqual(got, []string{"public", "private"}) {
		t.Errorf("expected default visibility [public private], got %v", got)
	}
	if c.Filter.OnlyNew == nil || *c.Filter.OnlyNew {
		t.Errorf("expected only_new to default to false, got %v", c.Filter.OnlyNew)
	}
	if c.Config.FetchAvatar == nil || !*c.Config.FetchAvatar {
		t.Errorf("expected fetch_avatar to default to true")
	}
	if c.Config.LFS == nil || !*c.Config.LFS {
		t.Errorf("expected lfs to default to true")
	}
	if c.Config.Wiki.Exclude == nil || *c.Config.Wiki.Exclude {
		t.Errorf("expected wiki.exclude to default to false")
	}
	if c.Config.Releases.Exclude == nil || *c.Config.Releases.Exclude {
		t.Errorf("expected releases.exclude to default to false")
	}
	if c.Config.Releases.Assets.Exclude == nil || *c.Config.Releases.Assets.Exclude {
		t.Errorf("expected releases.assets.exclude to default to false")
	}
	if c.Config.Releases.Assets.MaxSize == nil {
		t.Fatalf("expected releases.assets.max_size to have a default")
	}
	if got := utils.ConvertToBytes(*c.Config.Releases.Assets.MaxSize); got != 1<<30 {
		t.Errorf("expected the default max asset size to be 1 GiB, got %q = %d bytes", *c.Config.Releases.Assets.MaxSize, got)
	}
	if c.Groups == nil || c.Destinations == nil || c.Sources == nil {
		t.Errorf("expected nil slices to be replaced with empty slices")
	}
}

func TestPopulateDefault_SourceTimeoutsAreSecondsWithTenMinuteDefault(t *testing.T) {
	c := Configuration{Sources: []ConfigSource{
		{ID: "github"},
		{ID: "huggingface", Timeout: 30},
	}}
	c.PopulateDefault()

	if got := c.Sources[0].Timeout; got != 10*time.Minute {
		t.Errorf("expected default source timeout of 10m, got %v", got)
	}
	if got := c.Sources[1].Timeout; got != 30*time.Second {
		t.Errorf("expected timeout 30 to mean 30s, got %v", got)
	}
}

func TestPopulateDefault_DestinationTimeouts(t *testing.T) {
	c := Configuration{
		Sources: []ConfigSource{{ID: "github"}, {ID: "gitlab"}},
		Destinations: []ConfigDestination{
			{ID: "gitlab"},
			{ID: "gitea", Timeout: 45, GitTimeout: 5},
		},
	}
	c.PopulateDefault()

	if got := c.Destinations[0].Timeout; got != 10*time.Minute {
		t.Errorf("expected default destination timeout of 10m, got %v", got)
	}
	if got := c.Destinations[0].GitTimeout; got != time.Hour {
		t.Errorf("expected default git timeout of 1h, got %v", got)
	}
	if got := c.Destinations[1].Timeout; got != 45*time.Second {
		t.Errorf("expected timeout 45 to mean 45s, got %v", got)
	}
	if got := c.Destinations[1].GitTimeout; got != 5*time.Second {
		t.Errorf("expected git_timeout 5 to mean 5s, got %v", got)
	}
}

func TestPopulateDefault_MoreDestinationsThanSourcesDoesNotPanic(t *testing.T) {
	c := Configuration{
		Sources:      []ConfigSource{{ID: "github"}},
		Destinations: []ConfigDestination{{ID: "gitlab"}, {ID: "dufs"}, {ID: "gitea"}},
	}

	c.PopulateDefault()

	if got := c.Destinations[2].Timeout; got != 10*time.Minute {
		t.Errorf("expected every destination to receive defaults, got timeout %v for the last one", got)
	}
}

func TestPopulateDefault_GitlabSourceBaseURL(t *testing.T) {
	c := Configuration{Sources: []ConfigSource{
		{ID: "gitlab"},
		{ID: "gitlab-custom", BaseURL: "https://gitlab.example.com"},
		{ID: "github"},
	}}
	c.PopulateDefault()

	if got := c.Sources[0].BaseURL; got != "https://gitlab.com" {
		t.Errorf("expected gitlab source to default to https://gitlab.com, got %q", got)
	}
	if got := c.Sources[1].BaseURL; got != "https://gitlab.example.com" {
		t.Errorf("expected explicit base url to be kept, got %q", got)
	}
	if got := c.Sources[2].BaseURL; got != "" {
		t.Errorf("expected non-gitlab source to keep an empty base url, got %q", got)
	}
}

func TestPopulateDefault_GroupDefaults(t *testing.T) {
	c := Configuration{
		Sources: []ConfigSource{{ID: "github"}},
		Groups: []ConfigGroup{
			{Source: "github", Username: "octo"},
			{Source: "github", Username: "octo", GiteaUsername: "octo-mirror", Skip: intPtr(3)},
		},
	}
	c.PopulateDefault()

	if c.Groups[0].Skip == nil || *c.Groups[0].Skip != 0 {
		t.Errorf("expected skip to default to 0, got %v", c.Groups[0].Skip)
	}
	if got := c.Groups[0].GiteaUsername; got != "octo" {
		t.Errorf("expected gitea_username to default to username, got %q", got)
	}
	if got := *c.Groups[1].Skip; got != 3 {
		t.Errorf("expected explicit skip to be kept, got %d", got)
	}
	if got := c.Groups[1].GiteaUsername; got != "octo-mirror" {
		t.Errorf("expected explicit gitea_username to be kept, got %q", got)
	}
}

func TestPopulateDefault_CascadesFromGlobalThroughSourceAndGroupToRepository(t *testing.T) {
	c := Configuration{
		Config: ConfigRepo{Destination: strPtr("gitlab"), LFS: boolPtr(true)},
		Filter: ConfigFilter{Visibility: []string{"public"}},
		Sources: []ConfigSource{
			{
				ID:     "github",
				Config: ConfigRepo{LFS: boolPtr(false)},
				Filter: ConfigFilter{Archived: boolPtr(true)},
			},
		},
		Groups: []ConfigGroup{
			{
				Source:   "github",
				Username: "octo",
				Config:   ConfigRepo{Wiki: ConfigRepoWiki{Exclude: boolPtr(true)}},
				Filter:   ConfigFilter{Forked: boolPtr(false)},
				Repositories: []ConfigRepository{
					{Name: "inherits"},
					{
						Name:       "overrides",
						ConfigRepo: ConfigRepo{Wiki: ConfigRepoWiki{Exclude: boolPtr(false)}, LFS: boolPtr(true)},
						Filter:     ConfigFilter{Archived: boolPtr(false)},
					},
				},
			},
		},
	}
	c.PopulateDefault()

	group := c.Groups[0]
	if *group.Config.LFS != false {
		t.Errorf("group should inherit lfs=false from its source")
	}
	if *group.Config.Destination != "gitlab" {
		t.Errorf("group should inherit destination from the global config via the source")
	}
	if *group.Filter.Archived != true {
		t.Errorf("group should inherit archived=true filter from its source")
	}
	if !reflect.DeepEqual(group.Filter.Visibility, []string{"public"}) {
		t.Errorf("group should inherit global visibility, got %v", group.Filter.Visibility)
	}

	inherits := group.Repositories[0]
	if *inherits.LFS != false || *inherits.Wiki.Exclude != true || *inherits.Destination != "gitlab" {
		t.Errorf("repository should inherit lfs=false, wiki.exclude=true and destination=gitlab, got lfs=%v wiki=%v dest=%v",
			*inherits.LFS, *inherits.Wiki.Exclude, *inherits.Destination)
	}
	if *inherits.Filter.Archived != true || *inherits.Filter.Forked != false {
		t.Errorf("repository should inherit filters archived=true and forked=false")
	}
	if *inherits.Releases.Assets.MaxSize != *c.Config.Releases.Assets.MaxSize {
		t.Errorf("repository should inherit the global default max asset size")
	}

	overrides := group.Repositories[1]
	if *overrides.LFS != true || *overrides.Wiki.Exclude != false {
		t.Errorf("explicit repository config must not be overwritten, got lfs=%v wiki=%v", *overrides.LFS, *overrides.Wiki.Exclude)
	}
	if *overrides.Filter.Archived != false {
		t.Errorf("explicit repository filter must not be overwritten")
	}
}

func TestPopulateDefault_GroupWithUnknownSourceStillGetsBasicDefaults(t *testing.T) {
	c := Configuration{Groups: []ConfigGroup{{Source: "nope", Username: "octo"}}}
	c.PopulateDefault()

	if c.Groups[0].Skip == nil || c.Groups[0].GiteaUsername != "octo" {
		t.Fatalf("expected skip and gitea_username defaults even when the source is unknown")
	}
}

// --- lookups ---

func TestConfiguration_GetSource(t *testing.T) {
	c := Configuration{Sources: []ConfigSource{{ID: "github"}, {ID: "gitlab-x"}}}

	if got := c.GetSource("gitlab-x"); got == nil || got.ID != "gitlab-x" {
		t.Fatalf("expected to find gitlab-x, got %v", got)
	}
	if got := c.GetSource("missing"); got != nil {
		t.Fatalf("expected nil for unknown source, got %v", got)
	}
}

func TestConfiguration_GetDestination(t *testing.T) {
	c := Configuration{Destinations: []ConfigDestination{{ID: "gitlab"}, {ID: "dufs"}}}

	if got := c.GetDestination("dufs"); got == nil || got.ID != "dufs" {
		t.Fatalf("expected to find dufs, got %v", got)
	}
	if got := c.GetDestination("missing"); got != nil {
		t.Fatalf("expected nil for unknown destination, got %v", got)
	}
}

func TestConfigGroup_GetConfig(t *testing.T) {
	group := ConfigGroup{
		Config: ConfigRepo{LFS: boolPtr(true)},
		Repositories: []ConfigRepository{
			{Name: "MyRepo", ConfigRepo: ConfigRepo{LFS: boolPtr(false)}},
		},
	}

	if got := group.GetConfig("myrepo"); got.LFS == nil || *got.LFS != false {
		t.Errorf("expected repository-specific config (case-insensitive match), got %v", got.LFS)
	}
	if got := group.GetConfig("other"); got.LFS == nil || *got.LFS != true {
		t.Errorf("expected group config for unknown repository, got %v", got.LFS)
	}
}

// --- DefaultFrom ---

func fullRepoConfig(tag string) ConfigRepo {
	n := 1
	if tag == "b" {
		n = 2
	}
	return ConfigRepo{
		Retries:     intPtr(n),
		FetchAvatar: boolPtr(tag == "a"),
		Destination: strPtr("dest-" + tag),
		LFS:         boolPtr(tag == "a"),
		Wiki:        ConfigRepoWiki{Exclude: boolPtr(tag == "a")},
		Releases: ConfigRepoReleases{
			Exclude: boolPtr(tag == "a"),
			Assets: ConfigRepoAssets{
				Destination: strPtr("assets-" + tag),
				Exclude:     boolPtr(tag == "a"),
				MaxSize:     strPtr("size-" + tag),
			},
		},
	}
}

func TestConfigRepo_DefaultFrom_FillsEveryUnsetField(t *testing.T) {
	var c ConfigRepo
	c.DefaultFrom(fullRepoConfig("a"))

	if !reflect.DeepEqual(c, fullRepoConfig("a")) {
		t.Fatalf("expected every field to be copied from the default:\n got %+v\nwant %+v", c, fullRepoConfig("a"))
	}
}

func TestConfigRepo_DefaultFrom_KeepsEverySetField(t *testing.T) {
	c := fullRepoConfig("b")
	c.DefaultFrom(fullRepoConfig("a"))

	if !reflect.DeepEqual(c, fullRepoConfig("b")) {
		t.Fatalf("expected no field to be overwritten:\n got %+v\nwant %+v", c, fullRepoConfig("b"))
	}
}

func fullFilter(tag string) ConfigFilter {
	n := 1
	if tag == "b" {
		n = 2
	}
	return ConfigFilter{
		OnlyNew:        boolPtr(tag == "a"),
		Visibility:     []string{"vis-" + tag},
		Archived:       boolPtr(tag == "a"),
		Empty:          boolPtr(tag == "a"),
		HasDescription: boolPtr(tag == "a"),
		License:        []string{"lic-" + tag},
		Topics:         []string{"topic-" + tag},
		Pages:          boolPtr(tag == "a"),
		Discussions:    boolPtr(tag == "a"),
		Forked:         boolPtr(tag == "a"),
		NameRegex:      strPtr("re-" + tag),
		Language:       []string{"lang-" + tag},
		Stars:          ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		Watchers:       ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		Forks:          ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		Branches:       ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		Tags:           ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		CreatedAt:      ConfigFilterMinMax[string]{Min: strPtr("cmin-" + tag), Max: strPtr("cmax-" + tag)},
		Updated:        ConfigFilterMinMax[string]{Min: strPtr("umin-" + tag), Max: strPtr("umax-" + tag)},
		Size:           ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		Releases:       ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		Commits:        ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		Issues: ConfigFilterIssues{
			Enabled: boolPtr(tag == "a"),
			All:     ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
			Open:    ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
			Closed:  ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		},
		PullRequests: ConfigFilterPullRequests{
			All:    ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
			Open:   ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
			Closed: ConfigFilterMinMax[int]{Min: intPtr(n), Max: intPtr(n * 10)},
		},
	}
}

func TestConfigFilter_DefaultFrom_FillsEveryUnsetField(t *testing.T) {
	var c ConfigFilter
	c.DefaultFrom(fullFilter("a"))

	if !reflect.DeepEqual(c, fullFilter("a")) {
		t.Fatalf("expected every field to be copied from the default:\n got %+v\nwant %+v", c, fullFilter("a"))
	}
}

func TestConfigFilter_DefaultFrom_KeepsEverySetField(t *testing.T) {
	c := fullFilter("b")
	c.DefaultFrom(fullFilter("a"))

	if !reflect.DeepEqual(c, fullFilter("b")) {
		t.Fatalf("expected no field to be overwritten:\n got %+v\nwant %+v", c, fullFilter("b"))
	}
}

// --- TLS ---

func TestConfigDestination_LoadCABundle(t *testing.T) {
	t.Run("empty path loads nothing", func(t *testing.T) {
		got, err := ConfigDestination{}.LoadCABundle()
		if err != nil || got != nil {
			t.Fatalf("expected (nil, nil), got (%v, %v)", got, err)
		}
	})

	t.Run("reads the file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "ca.pem")
		if err := os.WriteFile(path, []byte("PEMDATA"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := ConfigDestination{CACert: path}.LoadCABundle()
		if err != nil || string(got) != "PEMDATA" {
			t.Fatalf("expected file content, got (%q, %v)", got, err)
		}
	})

	t.Run("missing file is an error", func(t *testing.T) {
		_, err := ConfigDestination{CACert: filepath.Join(t.TempDir(), "missing.pem")}.LoadCABundle()
		if err == nil {
			t.Fatal("expected an error for a missing CA file")
		}
	})
}

func TestConfigDestination_TLSConfig_IgnoreTLS(t *testing.T) {
	if cfg := (ConfigDestination{IgnoreTLS: true}).TLSConfig(); !cfg.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify when ignore_tls is set")
	}
	cfg := (ConfigDestination{}).TLSConfig()
	if cfg.InsecureSkipVerify {
		t.Fatal("expected certificate verification to stay enabled by default")
	}
	if cfg.RootCAs != nil {
		t.Fatal("expected the system roots (nil RootCAs) when no ca_cert is configured")
	}
}

func TestConfigDestination_TLSConfig_TrustsConfiguredCA(t *testing.T) {
	path, caCert := writeCAPEM(t)

	cfg := (ConfigDestination{CACert: path}).TLSConfig()
	if cfg.RootCAs == nil {
		t.Fatal("expected RootCAs to be populated from ca_cert")
	}
	if _, err := caCert.Verify(x509.VerifyOptions{Roots: cfg.RootCAs}); err != nil {
		t.Fatalf("expected the configured CA to be trusted by the TLS config: %v", err)
	}
}

func TestConfigDestination_TLSConfig_IgnoresUnreadableOrInvalidBundle(t *testing.T) {
	missing := (ConfigDestination{CACert: filepath.Join(t.TempDir(), "missing.pem")}).TLSConfig()
	if missing.RootCAs != nil {
		t.Error("expected a missing ca_cert to fall back to system roots")
	}

	garbage := filepath.Join(t.TempDir(), "garbage.pem")
	if err := os.WriteFile(garbage, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg := (ConfigDestination{CACert: garbage}).TLSConfig(); cfg.RootCAs != nil {
		t.Error("expected an invalid ca_cert to fall back to system roots")
	}
}

// --- Validate ---

func populated(c Configuration) *Configuration {
	c.PopulateDefault()
	return &c
}

func TestValidate_AcceptsValidConfiguration(t *testing.T) {
	if err := populated(validConfig()).Validate(); err != nil {
		t.Fatalf("expected a valid configuration, got: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *Configuration)
		wantErr string
	}{
		{
			name:    "source with invalid base url",
			mutate:  func(c *Configuration) { c.Sources[0].BaseURL = "http://[::1" },
			wantErr: "invalid base url",
		},
		{
			name:    "destination with invalid url",
			mutate:  func(c *Configuration) { c.Destinations[0].URL = "http://[::1" },
			wantErr: "invalid base url",
		},
		{
			name:    "group references unknown source",
			mutate:  func(c *Configuration) { c.Groups[0].Source = "bitbucket" },
			wantErr: "source bitbucket does not exist",
		},
		{
			name:    "group without username",
			mutate:  func(c *Configuration) { c.Groups[0].Username = "" },
			wantErr: "username is required",
		},
		{
			name:    "negative gitlab group id",
			mutate:  func(c *Configuration) { c.Groups[0].GitlabGroupID = -1 },
			wantErr: "gitlab_group_id is invalid",
		},
		{
			name:    "repository without name for a non-git source",
			mutate:  func(c *Configuration) { c.Groups[0].Repositories[0].Name = "" },
			wantErr: "name is required",
		},
		{
			name: "repository without destination",
			mutate: func(c *Configuration) {
				c.Config.Destination = nil
			},
			wantErr: "repository destination is required",
		},
		{
			name:    "repository with unknown destination",
			mutate:  func(c *Configuration) { c.Groups[0].Repositories[0].Destination = strPtr("nowhere") },
			wantErr: "destination nowhere does not exist",
		},
		{
			name: "repository without asset destination",
			mutate: func(c *Configuration) {
				c.Config.Releases.Assets.Destination = nil
			},
			wantErr: "asset destination is required",
		},
		{
			name: "repository with unknown asset destination",
			mutate: func(c *Configuration) {
				c.Groups[0].Repositories[0].Releases.Assets.Destination = strPtr("nowhere")
			},
			wantErr: "does not exist",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.mutate(&c)
			c.PopulateDefault()

			err := c.Validate()
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestValidate_GitSourceRepositoriesNeedNoName(t *testing.T) {
	c := validConfig()
	c.Groups = append(c.Groups, ConfigGroup{
		Source:   "git-proxmox",
		Username: "proxmox",
		Repositories: []ConfigRepository{
			{URL: "https://git.proxmox.com/git/lxc.git"},
		},
	})

	if err := populated(c).Validate(); err != nil {
		t.Fatalf("expected repositories of a git source to be valid without a name, got: %v", err)
	}
}

func TestValidate_CACert(t *testing.T) {
	t.Run("valid bundle", func(t *testing.T) {
		path, _ := writeCAPEM(t)
		c := validConfig()
		c.Destinations[0].CACert = path
		if err := populated(c).Validate(); err != nil {
			t.Fatalf("expected a valid CA bundle to be accepted, got: %v", err)
		}
	})

	t.Run("unreadable bundle", func(t *testing.T) {
		c := validConfig()
		c.Destinations[0].CACert = filepath.Join(t.TempDir(), "missing.pem")
		err := populated(c).Validate()
		if err == nil || !strings.Contains(err.Error(), "cannot read ca_cert") {
			t.Fatalf("expected a cannot-read error, got: %v", err)
		}
	})

	t.Run("bundle without certificates", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.pem")
		if err := os.WriteFile(path, []byte("nothing here"), 0o600); err != nil {
			t.Fatal(err)
		}
		c := validConfig()
		c.Destinations[0].CACert = path
		err := populated(c).Validate()
		if err == nil || !strings.Contains(err.Error(), "no valid certificates") {
			t.Fatalf("expected a no-valid-certificates error, got: %v", err)
		}
	})
}
