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

func TestMaterializeRejectsRemote(t *testing.T) {
	_, err := Materialize(source.Request{
		SourceID: "src-remote",
		URI:      "git+https://example.com/team/weave.git",
		CacheDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected remote source to be rejected in the local provider slice")
	}
}
