package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prulloac/weave-git/internal/source"
)

func commitAll(t *testing.T, root, message string) string {
	t.Helper()
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", message)
	return strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
}

func TestDiffChanges(t *testing.T) {
	root := gitFixture(t, map[string]string{
		"a.md":   "alpha\n",
		"old.md": "rename me\n",
	})
	from := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("alpha changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "mv", "old.md", "renamed.md")
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "logo.bin"), []byte{0, 1, 2, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	to := commitAll(t, root, "two")

	response, err := Diff(source.DiffRequest{
		V: source.ProtocolVersion, Type: "source_diff_request",
		SourceID: "src-test", URI: root, From: from, To: to,
		Options: source.DiffOptions{Format: "changes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.From != from || response.To != to {
		t.Fatalf("resolved refs = %s..%s, want %s..%s", response.From, response.To, from, to)
	}
	kinds := make(map[string]string)
	for _, change := range response.Changes {
		kinds[change.Path] = change.Kind
		if change.Kind == "binary" {
			t.Fatal("unexpected binary kind")
		}
		if change.Patch != "" {
			t.Fatalf("changes format must not carry a patch for %s", change.Path)
		}
	}
	want := map[string]string{
		"a.md":       "modified",
		"b.md":       "added",
		"logo.bin":   "added",
		"renamed.md": "renamed",
	}
	for path, kind := range want {
		if kinds[path] != kind {
			t.Fatalf("%s = %q, want %q (all: %+v)", path, kinds[path], kind, response.Changes)
		}
	}
}

func TestDiffPatchAndBinary(t *testing.T) {
	root := gitFixture(t, map[string]string{
		"a.md":     "alpha\n",
		"logo.bin": string([]byte{0, 1, 2, 0}),
	})
	from := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("alpha changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "logo.bin"), []byte{0, 3, 4, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	to := commitAll(t, root, "two")

	response, err := Diff(source.DiffRequest{
		V: source.ProtocolVersion, Type: "source_diff_request",
		SourceID: "src-test", URI: root, From: from, To: to,
		Options: source.DiffOptions{Format: "patch"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range response.Changes {
		switch change.Path {
		case "a.md":
			if change.Binary || change.Patch == "" || !strings.Contains(change.Patch, "@@") {
				t.Fatalf("unexpected text patch: %+v", change)
			}
		case "logo.bin":
			if !change.Binary || change.Patch != "" {
				t.Fatalf("binary change should have no patch: %+v", change)
			}
		}
	}
}

func TestDiffDeterministic(t *testing.T) {
	root := gitFixture(t, map[string]string{"a.md": "alpha\n"})
	from := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	to := commitAll(t, root, "two")
	request := source.DiffRequest{
		V: source.ProtocolVersion, Type: "source_diff_request",
		SourceID: "src-test", URI: root, From: from, To: to,
		Options: source.DiffOptions{Format: "patch"},
	}
	first, err := Diff(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Diff(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Changes) != len(second.Changes) {
		t.Fatal("diff is not deterministic")
	}
	for index := range first.Changes {
		if first.Changes[index] != second.Changes[index] {
			t.Fatalf("diff is not deterministic at %d", index)
		}
	}
}

func TestDiffUnknownRef(t *testing.T) {
	root := gitFixture(t, map[string]string{"a.md": "alpha\n"})
	commit := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	_, err := Diff(source.DiffRequest{
		V: source.ProtocolVersion, Type: "source_diff_request",
		SourceID: "src-test", URI: root, From: commit, To: "deadbeef",
	})
	if err == nil {
		t.Fatal("expected an error for an unknown ref")
	}
}

func TestParseNameStatus(t *testing.T) {
	data := []byte("M\x00a.md\x00A\x00b.md\x00D\x00c.md\x00R100\x00old.md\x00new.md\x00")
	changes, err := parseNameStatus(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 4 {
		t.Fatalf("changes = %+v", changes)
	}
	if changes[3].Kind != "renamed" || changes[3].PreviousPath != "old.md" || changes[3].Path != "new.md" {
		t.Fatalf("rename = %+v", changes[3])
	}
}
