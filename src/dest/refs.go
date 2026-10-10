package dest

import (
	"errors"
	"fmt"
	"main/src/configuration"
	"main/src/sources"
	"sort"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
)

// RefDifference is a source branch or tag that the destination lacks or holds at another commit.
type RefDifference struct {
	Name        string
	Source      plumbing.Hash
	Destination plumbing.Hash // zero when the destination does not have the ref at all
}

func (d RefDifference) String() string {
	if d.Destination.IsZero() {
		return fmt.Sprintf("%s: source %s, destination missing", d.Name, shortHash(d.Source))
	}
	return fmt.Sprintf("%s: source %s, destination %s", d.Name, shortHash(d.Source), shortHash(d.Destination))
}

func shortHash(h plumbing.Hash) string {
	return h.String()[:7]
}

// ListRemoteRefs returns the branches and tags a remote repository advertises, keyed by full ref name,
// without cloning it: the equivalent of `git ls-remote --heads --tags`.
func ListRemoteRefs(url string, auth transport.AuthMethod, insecureSkipTLS bool, caBundle []byte) (map[string]plumbing.Hash, error) {
	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{Name: "origin", URLs: []string{url}})
	refs, err := remote.List(&git.ListOptions{
		Auth:            auth,
		InsecureSkipTLS: insecureSkipTLS,
		CABundle:        caBundle,
	})
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		// A repository without any commit advertises nothing; that is zero refs, not a failure.
		return map[string]plumbing.Hash{}, nil
	}
	if err != nil {
		return nil, err
	}

	out := make(map[string]plumbing.Hash)
	for _, ref := range refs {
		if ref.Type() != plumbing.HashReference {
			continue // HEAD and other symbolic refs
		}
		if !ref.Name().IsBranch() && !ref.Name().IsTag() {
			continue
		}
		out[ref.Name().String()] = ref.Hash()
	}
	return out, nil
}

// RefDifferences lists the source refs that are missing from the destination or point at another
// commit, sorted by name. Refs only the destination has are ignored: the sync never deletes.
func RefDifferences(source, destination map[string]plumbing.Hash) []RefDifference {
	diffs := make([]RefDifference, 0)
	for name, srcHash := range source {
		if dstHash, ok := destination[name]; !ok || dstHash != srcHash {
			diffs = append(diffs, RefDifference{Name: name, Source: srcHash, Destination: dstHash})
		}
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Name < diffs[j].Name })
	return diffs
}

// IsUpToDate reports whether the destination repository at dstURL already holds every branch and tag
// of the source at the same commits, in which case cloning and pushing can be skipped. The differences
// found are returned so they can be reported.
func (r *Repository) IsUpToDate(source sources.Source, dstURL string, dstCfg configuration.ConfigDestination) (bool, []RefDifference, error) {
	if dstURL == "" {
		return false, nil, fmt.Errorf("destination has no remote url for %s", r.Name)
	}
	if err := r.useDestinationTransport(dstCfg); err != nil {
		return false, nil, err
	}

	srcRefs, err := ListRemoteRefs(r.Remote.URL, sourceAuth(source), r.DstIgnoreTLS, r.DstCABundle)
	if err != nil {
		return false, nil, fmt.Errorf("listing source refs: %w", err)
	}
	dstRefs, err := ListRemoteRefs(dstURL, nil, r.DstIgnoreTLS, r.DstCABundle)
	if err != nil {
		return false, nil, fmt.Errorf("listing destination refs: %w", err)
	}

	diffs := RefDifferences(srcRefs, dstRefs)
	return len(diffs) == 0, diffs, nil
}
