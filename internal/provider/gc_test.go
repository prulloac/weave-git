package provider

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeCacheSource(t *testing.T, cache, id, commit string) string {
	t.Helper()
	base := filepath.Join(cache, "sources", id)
	worktree := filepath.Join(base, "worktree")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("# Fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeState(base, state{Commit: commit, URI: "https://example.com/repo.git"}); err != nil {
		t.Fatal(err)
	}
	return base
}

func TestCollectGarbageRemovesIncompleteAndStale(t *testing.T) {
	cache := t.TempDir()
	active := writeCacheSource(t, cache, "src-active", "8f3c2a1d4b5e6f708192a3b4c5d6e7f8091a2b3c")
	incomplete := filepath.Join(cache, "sources", "src-incomplete")
	if err := os.MkdirAll(filepath.Join(incomplete, "worktree"), 0o755); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(active, "worktree.stage-123")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(active, "worktree.old")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := CollectGarbage(GCOptions{CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(incomplete); !os.IsNotExist(err) {
		t.Fatalf("incomplete source not removed: %v", err)
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("stale stage not removed: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old worktree not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(active, "worktree")); err != nil {
		t.Fatalf("active source removed: %v", err)
	}
	if len(result.Removed) != 3 {
		t.Fatalf("removed = %+v", result.Removed)
	}
}

func TestCollectGarbageHonorsKeep(t *testing.T) {
	cache := t.TempDir()
	kept := writeCacheSource(t, cache, "src-kept", "8f3c2a1d4b5e6f708192a3b4c5d6e7f8091a2b3c")
	removed := writeCacheSource(t, cache, "src-removed", "8f3c2a1d4b5e6f708192a3b4c5d6e7f8091a2b3c")
	if _, err := CollectGarbage(GCOptions{CacheDir: cache, Keep: []string{"src-kept"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("kept source removed: %v", err)
	}
	if _, err := os.Stat(removed); !os.IsNotExist(err) {
		t.Fatalf("unkept source not removed: %v", err)
	}
}

func TestCollectGarbageDryRun(t *testing.T) {
	cache := t.TempDir()
	incomplete := filepath.Join(cache, "sources", "src-dry")
	if err := os.MkdirAll(incomplete, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := CollectGarbage(GCOptions{CacheDir: cache, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 1 {
		t.Fatalf("removed = %+v", result.Removed)
	}
	if _, err := os.Stat(incomplete); err != nil {
		t.Fatalf("dry run removed the directory: %v", err)
	}
}

func TestCollectGarbagePrunesMirrors(t *testing.T) {
	if _, err := gitBytes("", "version"); err != nil {
		t.Skip("git is not installed")
	}
	cache := t.TempDir()
	mirror := filepath.Join(cache, "git", "example.com", "team", "repo.git")
	if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := gitBytes("", "init", "--bare", "--quiet", mirror); err != nil {
		t.Fatal(err)
	}
	result, err := CollectGarbage(GCOptions{CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pruned) != 1 || result.Pruned[0] != "git/example.com/team/repo.git" {
		t.Fatalf("pruned = %+v", result.Pruned)
	}
}

func archiveWithFiles(t *testing.T, files map[string]int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for name, size := range files {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(size), Typeflag: tar.TypeReg}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(bytes.Repeat([]byte("x"), size)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestExtractRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"../escape.md", "/absolute.md"} {
		var buffer bytes.Buffer
		writer := tar.NewWriter(&buffer)
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := extract(buffer.Bytes(), t.TempDir()); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}

func TestExtractEnforcesLimits(t *testing.T) {
	limits := snapshotLimits{MaxFileBytes: 1024, MaxProjectBytes: 1500, MaxFiles: 2}
	tests := []struct {
		name  string
		files map[string]int
		ok    bool
	}{
		{"within", map[string]int{"a.md": 100, "b.md": 200}, true},
		{"file too large", map[string]int{"big.md": 2048}, false},
		{"project too large", map[string]int{"a.md": 800, "b.md": 800}, false},
		{"too many files", map[string]int{"a.md": 1, "b.md": 1, "c.md": 1}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			destination := t.TempDir()
			_, err := extractWithLimits(archiveWithFiles(t, test.files), destination, limits)
			if test.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("expected a limits error")
			}
		})
	}
}
