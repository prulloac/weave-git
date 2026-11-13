package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/prulloac/weave-git/internal/source"
)

var credentialPattern = regexp.MustCompile(`://[^/@\s]+@`)

// resolveRemote fetches a remote source into a bare mirror and resolves its commit.
func resolveRemote(request source.Request, loc location) (string, string, []string, error) {
	base := sourceBase(request)
	saved, hasState := readState(base)
	mirror, err := ensureMirror(request.CacheDir, loc)
	if err != nil {
		return "", "", nil, err
	}
	ref := strings.TrimSpace(request.Ref)
	if hasState && saved.Commit != "" && fullCommitRef.MatchString(ref) && strings.EqualFold(ref, saved.Commit) && worktreeExists(base) {
		return saved.Commit, mirror, nil, nil
	}
	commit, err := fetchRemote(mirror, loc.FetchURI, ref, request.Options.Depth)
	if err == nil {
		return commit, mirror, nil, nil
	}
	if hasState && saved.Commit != "" && cachedReusable(saved, loc.URI, ref, mirror) {
		if !request.Options.Permissive {
			return "", mirror, nil, err
		}
		warning := fmt.Sprintf("remote unavailable; using cached commit %s", saved.Commit)
		return saved.Commit, mirror, []string{warning}, nil
	}
	return "", mirror, nil, err
}

func sourceBase(request source.Request) string {
	return filepath.Join(request.CacheDir, "sources", request.SourceID)
}

func worktreeExists(base string) bool {
	info, err := os.Stat(filepath.Join(base, "worktree"))
	return err == nil && info.IsDir()
}

func cachedReusable(saved state, uri, ref, mirror string) bool {
	if saved.URI != "" && saved.URI != uri {
		return false
	}
	if normalizeRef(saved.Ref) != normalizeRef(ref) {
		return false
	}
	return hasCommit(mirror, saved.Commit)
}

func ensureMirror(cacheDir string, loc location) (string, error) {
	mirror, err := loc.mirrorPath(cacheDir)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(mirror); err == nil && info.IsDir() {
		return mirror, nil
	}
	if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
		return "", err
	}
	if _, err := gitBytes("", "init", "--bare", "--quiet", mirror); err != nil {
		return "", err
	}
	return mirror, nil
}

func fetchRemote(mirror, uri, ref string, depth int) (string, error) {
	target := normalizeRef(ref)
	if hexCommitishRef.MatchString(target) {
		if commit, ok := localCommit(mirror, target); ok {
			return commit, nil
		}
	}
	args := []string{"fetch", "--force", "--no-tags"}
	if depth > 0 {
		args = append(args, fmt.Sprintf("--depth=%d", depth))
	}
	args = append(args, uri, target)
	if _, err := gitBytes(mirror, args...); err != nil {
		if !hexCommitishRef.MatchString(target) || !strings.Contains(err.Error(), "couldn't find remote ref") {
			return "", err
		}
		fallback := []string{"fetch", "--force", "--no-tags"}
		if depth > 0 {
			fallback = append(fallback, fmt.Sprintf("--depth=%d", depth))
		}
		fallback = append(fallback, uri)
		if _, fallbackErr := gitBytes(mirror, fallback...); fallbackErr != nil {
			return "", err
		}
	}
	if hexCommitishRef.MatchString(target) {
		if commit, ok := localCommit(mirror, target); ok {
			return commit, nil
		}
	}
	return verifyCommit(mirror, "FETCH_HEAD")
}

func localCommit(mirror, ref string) (string, bool) {
	commit, err := verifyCommit(mirror, ref)
	if err != nil {
		return "", false
	}
	return commit, true
}

func verifyCommit(mirror, ref string) (string, error) {
	output, err := gitBytes(mirror, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func hasCommit(mirror, commit string) bool {
	if strings.TrimSpace(commit) == "" {
		return false
	}
	_, err := gitBytes(mirror, "cat-file", "-e", commit+"^{commit}")
	return err == nil
}

func readState(base string) (state, bool) {
	data, err := os.ReadFile(filepath.Join(base, "state.json"))
	if err != nil {
		return state{}, false
	}
	var value state
	if err := json.Unmarshal(data, &value); err != nil {
		return state{}, false
	}
	return value, true
}

func redactCredentials(value string) string {
	return credentialPattern.ReplaceAllString(value, "://***@")
}
