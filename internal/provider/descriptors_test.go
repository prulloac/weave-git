package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prulloac/weave-git/internal/source"
)

func TestMaterializeEmitsDescriptors(t *testing.T) {
	root := gitFixture(t, map[string]string{
		"README.md":        "# Descriptor Fixture\n\nA descriptor repo.\n",
		"go.mod":           "module example.com/fixture\n\nrequire github.com/acme/lib v1.2.3\n",
		"web/app.ts":       "export const value = 1;\n",
		"web/package.json": `{"name":"@acme/web","dependencies":{"react":"^18"}}`,
	})
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "add project", "--allow-empty")
	head := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	response, err := Materialize(source.Request{
		SourceID: "src-descriptors",
		URI:      root,
		CacheDir: t.TempDir(),
		Options:  source.Options{History: true, HistoryLimit: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]source.Document{}
	for _, document := range response.Documents {
		byID[document.ID] = document
	}
	for _, id := range []string{"_repo", "_project", "_project/web", "_package/go/github-com-acme-lib", "_package/npm/react", "_commit/" + head} {
		if byID[id].ID == "" {
			t.Fatalf("missing descriptor %s: %+v", id, response.Documents)
		}
	}
	if byID["_repo"].Type != "Repository" || byID["_project/web"].Type != "Project" ||
		byID["_package/npm/react"].Type != "ExternalPackage" {
		t.Fatalf("unexpected types: %+v", byID)
	}
	if byID["_repo"].Consumes[0] != "README.md" || !strings.Contains(byID["_repo"].Body, "Descriptor Fixture") {
		t.Fatalf("repository descriptor = %+v", byID["_repo"])
	}
	if response.SourceMetadata["revision"] != head || response.SourceMetadata["provider"] != "git" {
		t.Fatalf("source metadata = %+v", response.SourceMetadata)
	}
	hasDepends := false
	hasParent := true
	for _, relation := range response.Relations {
		if relation.From == "_project/web" && relation.To == "_package/npm/react" && relation.Kind == "depends_on" {
			hasDepends = true
		}
		if relation.Kind == "parent_of" {
			hasParent = true
		}
	}
	if !hasDepends || !hasParent {
		t.Fatalf("relations = %+v", response.Relations)
	}
	if len(response.Repository.Name) == 0 || len(response.Projects) == 0 || len(response.Commits) == 0 {
		t.Fatal("legacy fields must remain populated")
	}
}

func TestProviderOptionsOverrideTypedOptions(t *testing.T) {
	root := gitFixture(t, map[string]string{"README.md": "# Options\n"})
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "add", name)
		runGit(t, root, "commit", "-q", "-m", name)
	}
	response, err := Materialize(source.Request{
		SourceID: "src-options",
		URI:      root,
		CacheDir: t.TempDir(),
		Options: source.Options{
			History: false,
			ProviderOptions: map[string]any{
				"history":       "true",
				"history_limit": "2",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Commits) != 2 {
		t.Fatalf("commits = %+v", response.Commits)
	}
}
