package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscoverProjects(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod": `module example.com/mono

go 1.27.3

require (
	github.com/acme/lib v1.2.3
	golang.org/x/text v0.14.0 // indirect
)

require github.com/single/dep v0.1.0
`,
		"web/package.json": `{
  "name": "@acme/web",
  "description": "Web app",
  "dependencies": {"react": "^18"},
  "devDependencies": {"vite": "^5"},
  "peerDependencies": {"react-dom": "^18"},
  "optionalDependencies": {"fsevents": "^2"}
}`,
		"crates/core/Cargo.toml": `[package]
name = "core-lib"
description = "Core library"

[dependencies]
serde = { version = "1.0", features = ["derive"] }
anyhow = "1"

[dev-dependencies]
tempfile = "3"
`,
		"py/pyproject.toml": `[project]
name = "py-lib"
description = "Python library"
dependencies = [
  "requests>=2.31",
  "httpx[socks]==0.27",
]

[project.optional-dependencies]
test = ["pytest>=8"]
`,
		"node_modules/ignored/package.json": `{"name":"ignored"}`,
		"vendor/ignored/go.mod":             "module ignored\n",
	})

	projects, warnings := discoverProjects(root)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v", warnings)
	}
	byPath := map[string]projectManifest{}
	for _, project := range projects {
		byPath[project.Path] = project
	}
	if len(projects) != 4 || byPath[""].Name != "example.com/mono" ||
		byPath["web"].Name != "@acme/web" ||
		byPath["crates/core"].Name != "core-lib" ||
		byPath["py"].Name != "py-lib" {
		t.Fatalf("projects = %+v", projects)
	}
	if byPath["web"].Description != "Web app" || byPath["crates/core"].Description != "Core library" {
		t.Fatalf("descriptions = %+v", byPath)
	}

	rootDeps := dependencyMap(byPath[""].Dependencies)
	if rootDeps["github.com/acme/lib"].Version != "v1.2.3" || rootDeps["github.com/acme/lib"].Scope != "runtime" {
		t.Fatalf("root dependency = %+v", rootDeps)
	}
	if rootDeps["golang.org/x/text"].Scope != "indirect" {
		t.Fatalf("indirect dependency = %+v", rootDeps)
	}
	if rootDeps["github.com/single/dep"].Version != "v0.1.0" {
		t.Fatalf("single dependency = %+v", rootDeps)
	}

	webDeps := dependencyMap(byPath["web"].Dependencies)
	if len(webDeps) != 4 || webDeps["react"].Scope != "runtime" ||
		webDeps["vite"].Scope != "development" || webDeps["react-dom"].Scope != "peer" ||
		webDeps["fsevents"].Scope != "optional" {
		t.Fatalf("web dependencies = %+v", webDeps)
	}

	cargoDeps := dependencyMap(byPath["crates/core"].Dependencies)
	if cargoDeps["serde"].Version != "1.0" || cargoDeps["serde"].Scope != "runtime" ||
		cargoDeps["tempfile"].Scope != "development" {
		t.Fatalf("cargo dependencies = %+v", cargoDeps)
	}

	pyDeps := dependencyMap(byPath["py"].Dependencies)
	if pyDeps["requests"].Version != ">=2.31" || pyDeps["httpx"].Version != "==0.27" ||
		pyDeps["pytest"].Scope != "optional" {
		t.Fatalf("python dependencies = %+v", pyDeps)
	}
}

func dependencyMap(values []dependencyManifest) map[string]dependencyManifest {
	result := make(map[string]dependencyManifest, len(values))
	for _, value := range values {
		result[value.Name] = value
	}
	return result
}
