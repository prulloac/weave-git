package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prulloac/weave-git/internal/source"
)

func TestMaterializeHistoryOptIn(t *testing.T) {
	root := gitFixture(t, map[string]string{"README.md": "# History\n"})
	for index, name := range []string{"one.md", "two.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", name)
		runGit(t, root, "commit", "-q", "-m", "commit "+string(rune('1'+index)))
	}
	head := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))

	off, err := Materialize(source.Request{
		SourceID: "src-history-off",
		URI:      root,
		CacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(off.Commits) != 0 {
		t.Fatalf("history should be opt-in: %+v", off.Commits)
	}

	on, err := Materialize(source.Request{
		SourceID: "src-history-on",
		URI:      root,
		CacheDir: t.TempDir(),
		Options:  source.Options{History: true, HistoryLimit: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(on.Commits) != 2 {
		t.Fatalf("commits = %+v", on.Commits)
	}
	if on.Commits[0].SHA != head {
		t.Fatalf("newest commit = %s, want %s", on.Commits[0].SHA, head)
	}
	if len(on.Commits[0].Parents) != 1 || on.Commits[0].Parents[0] != on.Commits[1].SHA {
		t.Fatalf("parent chain = %+v", on.Commits)
	}
	if on.Commits[0].Author != "Test" || on.Commits[0].Date == "" ||
		!strings.HasPrefix(on.Commits[0].Subject, "commit ") {
		t.Fatalf("commit metadata = %+v", on.Commits[0])
	}
}

func TestEnrichRepositoryHostURLs(t *testing.T) {
	commit := "8f3c2a1d4b5e6f708192a3b4c5d6e7f8091a2b3c"
	tests := []struct {
		uri       string
		hostKind  string
		webURL    string
		commitURL string
	}{
		{
			"git+https://github.com/acme/weave.git",
			"github", "https://github.com/acme/weave", "https://github.com/acme/weave/commit/" + commit,
		},
		{
			"git+https://gitlab.com/acme/weave.git",
			"gitlab", "https://gitlab.com/acme/weave", "https://gitlab.com/acme/weave/-/commit/" + commit,
		},
		{
			"git+https://bitbucket.org/acme/weave.git",
			"bitbucket", "https://bitbucket.org/acme/weave", "https://bitbucket.org/acme/weave/commits/" + commit,
		},
		{
			"git+https://git.example.com/team/weave.git",
			"generic", "https://git.example.com/team/weave", "https://git.example.com/team/weave/commit/" + commit,
		},
	}
	for _, test := range tests {
		loc, err := parseLocation(test.uri)
		if err != nil {
			t.Fatal(err)
		}
		repository := source.Repository{}
		enrichRepository(&repository, loc, commit)
		if repository.HostKind != test.hostKind || repository.WebURL != test.webURL || repository.CommitURL != test.commitURL {
			t.Fatalf("%s: %+v", test.uri, repository)
		}
	}
}

func TestEnrichRepositoryLocalHasNoURLs(t *testing.T) {
	loc, err := parseLocation(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := source.Repository{}
	enrichRepository(&repository, loc, "8f3c2a1d4b5e6f708192a3b4c5d6e7f8091a2b3c")
	if repository.HostKind != "" || repository.WebURL != "" || repository.CommitURL != "" {
		t.Fatalf("local repository gained URLs: %+v", repository)
	}
}
