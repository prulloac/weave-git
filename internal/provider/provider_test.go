package provider

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prulloac/weave-git/internal/source"
)

func gitFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "fixture")
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func TestMaterializeLocalSnapshot(t *testing.T) {
	root := gitFixture(t, map[string]string{
		"README.md": "# Weave fixture\n\nA small repository used by the provider tests.\n",
		"main.go":   "package main\n",
	})
	cache := t.TempDir()
	response, err := Materialize(source.Request{
		SourceID: "src-test",
		URI:      root,
		CacheDir: cache,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Snapshot.Commit == "" || !response.Snapshot.ReadOnly {
		t.Fatalf("unexpected snapshot: %+v", response.Snapshot)
	}
	if !strings.HasPrefix(response.Snapshot.Path, cache) {
		t.Fatalf("snapshot path %q is outside cache", response.Snapshot.Path)
	}
	if _, err := os.Stat(filepath.Join(response.Snapshot.Path, "main.go")); err != nil {
		t.Fatalf("materialized file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(response.Snapshot.Path, ".git")); !os.IsNotExist(err) {
		t.Fatalf("snapshot must not contain .git: %v", err)
	}
	if response.Repository.Title != "Weave fixture" ||
		response.Repository.Description != "A small repository used by the provider tests." {
		t.Fatalf("unexpected repository metadata: %+v", response.Repository)
	}
}

func TestMaterializeWorktree(t *testing.T) {
	root := gitFixture(t, map[string]string{"README.md": "# Fixture\n"})
	response, err := Materialize(source.Request{
		SourceID: "src-worktree",
		URI:      root,
		Options:  source.Options{Worktree: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Snapshot.Path != root || response.Snapshot.ReadOnly {
		t.Fatalf("unexpected worktree snapshot: %+v", response.Snapshot)
	}
}

func TestReadmeFrontmatterDescription(t *testing.T) {
	root := gitFixture(t, map[string]string{
		"README.md": "---\ntitle: Weave\ndescription: Local-first database.\n---\n\n# Weave\n\nLonger body.\n",
	})
	response, err := Materialize(source.Request{
		SourceID: "src-frontmatter",
		URI:      root,
		CacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Repository.Description != "Local-first database." {
		t.Fatalf("description = %q", response.Repository.Description)
	}
}

func TestMaterializeMissingReadme(t *testing.T) {
	root := gitFixture(t, map[string]string{"main.go": "package main\n"})
	response, err := Materialize(source.Request{
		SourceID: "src-no-readme",
		URI:      root,
		CacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Warnings) == 0 {
		t.Fatal("expected a missing README warning")
	}
	if response.Repository.Name != filepath.Base(root) {
		t.Fatalf("unexpected repository name: %+v", response.Repository)
	}
}

func bareFixture(t *testing.T, source string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	bare := filepath.Join(t.TempDir(), "repo.git")
	command := exec.Command("git", "clone", "--bare", "--quiet", source, bare)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git clone --bare: %v\n%s", err, output)
	}
	return bare
}

func fileURI(path string) string {
	return "git+file://" + filepath.ToSlash(path)
}

func TestMaterializeRemoteFileURI(t *testing.T) {
	root := gitFixture(t, map[string]string{
		"README.md": "# Remote fixture\n\nFetched through a bare mirror.\n",
		"main.go":   "package main\n",
	})
	bare := bareFixture(t, root)
	cache := t.TempDir()
	response, err := Materialize(source.Request{
		SourceID: "src-remote",
		URI:      fileURI(bare),
		CacheDir: cache,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Snapshot.Commit == "" || !response.Snapshot.ReadOnly {
		t.Fatalf("unexpected snapshot: %+v", response.Snapshot)
	}
	if !strings.HasPrefix(response.Snapshot.Path, cache) {
		t.Fatalf("snapshot path %q is outside cache", response.Snapshot.Path)
	}
	if _, err := os.Stat(filepath.Join(response.Snapshot.Path, "main.go")); err != nil {
		t.Fatalf("materialized file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(response.Snapshot.Path, ".git")); !os.IsNotExist(err) {
		t.Fatalf("snapshot must not contain .git: %v", err)
	}
	if response.Repository.Name != "repo" {
		t.Fatalf("repository name = %q", response.Repository.Name)
	}
	if response.Repository.Title != "Remote fixture" {
		t.Fatalf("repository title = %q", response.Repository.Title)
	}
	if _, err := os.Stat(filepath.Join(cache, "git")); err != nil {
		t.Fatalf("bare mirror missing: %v", err)
	}
}

func TestMaterializeRemoteReusesCacheOffline(t *testing.T) {
	root := gitFixture(t, map[string]string{"README.md": "# Offline\n"})
	bare := bareFixture(t, root)
	cache := t.TempDir()
	request := source.Request{
		SourceID: "src-offline",
		URI:      fileURI(bare),
		CacheDir: cache,
	}
	first, err := Materialize(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(bare); err != nil {
		t.Fatal(err)
	}
	request.Options.Permissive = true
	second, err := Materialize(request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Snapshot.Commit != first.Snapshot.Commit {
		t.Fatalf("cached commit = %q, want %q", second.Snapshot.Commit, first.Snapshot.Commit)
	}
	if second.Snapshot.Path != first.Snapshot.Path {
		t.Fatalf("worktree path changed: %q != %q", second.Snapshot.Path, first.Snapshot.Path)
	}
	if len(second.Warnings) == 0 || !strings.Contains(strings.Join(second.Warnings, " "), first.Snapshot.Commit) {
		t.Fatalf("expected a cached-commit warning: %+v", second.Warnings)
	}
}

func TestMaterializeRemoteStrictOfflineFails(t *testing.T) {
	root := gitFixture(t, map[string]string{"README.md": "# Offline\n"})
	bare := bareFixture(t, root)
	cache := t.TempDir()
	request := source.Request{
		SourceID: "src-strict",
		URI:      fileURI(bare),
		CacheDir: cache,
	}
	if _, err := Materialize(request); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(bare); err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(request); err == nil {
		t.Fatal("expected strict offline fetch to fail")
	}
}

func TestMaterializeRemoteWorktreeInvalid(t *testing.T) {
	root := gitFixture(t, map[string]string{"README.md": "# Remote\n"})
	bare := bareFixture(t, root)
	_, err := Materialize(source.Request{
		SourceID: "src-remote-worktree",
		URI:      fileURI(bare),
		CacheDir: t.TempDir(),
		Options:  source.Options{Worktree: true},
	})
	if err == nil {
		t.Fatal("expected remote worktree mode to fail")
	}
}

func TestMaterializeRemoteRefUpdateKeepsWorktree(t *testing.T) {
	root := gitFixture(t, map[string]string{"README.md": "# Before\n"})
	bare := bareFixture(t, root)
	branch := strings.TrimSpace(runGit(t, root, "symbolic-ref", "--short", "HEAD"))
	cache := t.TempDir()
	request := source.Request{
		SourceID: "src-update",
		URI:      fileURI(bare),
		Ref:      branch,
		CacheDir: cache,
	}
	first, err := Materialize(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# After\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-q", "-m", "update")
	runGit(t, root, "push", "--quiet", bare, branch)
	second, err := Materialize(request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Snapshot.Commit == first.Snapshot.Commit {
		t.Fatal("expected a new commit")
	}
	if second.Snapshot.Path != first.Snapshot.Path {
		t.Fatalf("worktree path changed: %q != %q", second.Snapshot.Path, first.Snapshot.Path)
	}
	data, err := os.ReadFile(filepath.Join(second.Snapshot.Path, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# After\n" {
		t.Fatalf("worktree was not updated: %q", data)
	}
	if second.Repository.Title != "After" {
		t.Fatalf("repository title = %q", second.Repository.Title)
	}
}

func TestMaterializeRemoteCommitAndDepth(t *testing.T) {
	root := gitFixture(t, map[string]string{"README.md": "# Commit\n"})
	bare := bareFixture(t, root)
	commit := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	abbreviated := commit[:10]
	for _, ref := range []string{commit, abbreviated} {
		response, err := Materialize(source.Request{
			SourceID: "src-commit-" + ref[:6],
			URI:      fileURI(bare),
			Ref:      ref,
			CacheDir: t.TempDir(),
			Options:  source.Options{Depth: 1},
		})
		if err != nil {
			t.Fatalf("ref %s: %v", ref, err)
		}
		if response.Snapshot.Commit != commit {
			t.Fatalf("ref %s resolved to %q, want %q", ref, response.Snapshot.Commit, commit)
		}
	}
}

func TestParseLocation(t *testing.T) {
	tests := []struct {
		uri    string
		remote bool
		host   string
		owner  string
		name   string
	}{
		{"git+https://git.example.com/team/weave.git", true, "git.example.com", "team", "weave"},
		{"git+ssh://git@git.example.com/team/weave.git", true, "git.example.com", "team", "weave"},
		{"git@git.example.com:team/weave.git", true, "git.example.com", "team", "weave"},
		{"git+file:///srv/repos/weave.git", true, "", "srv/repos", "weave"},
	}
	for _, test := range tests {
		loc, err := parseLocation(test.uri)
		if err != nil {
			t.Fatalf("%s: %v", test.uri, err)
		}
		if loc.Remote != test.remote || loc.Host != test.host || loc.Owner != test.owner || loc.Name != test.name {
			t.Fatalf("%s: %+v", test.uri, loc)
		}
	}
	local := t.TempDir()
	loc, err := parseLocation(local)
	if err != nil {
		t.Fatal(err)
	}
	if loc.Remote || loc.Name != filepath.Base(local) {
		t.Fatalf("local location: %+v", loc)
	}
}

func TestParseLocationRedactsPassword(t *testing.T) {
	loc, err := parseLocation("git+https://user:secret@example.com/team/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(loc.URI, "secret") {
		t.Fatalf("uri leaked a password: %s", loc.URI)
	}
}

func TestRedactCredentials(t *testing.T) {
	message := redactCredentials("fatal: unable to access 'https://user:token@example.com/team/repo.git'")
	if strings.Contains(message, "token") {
		t.Fatalf("credentials not redacted: %s", message)
	}
}
