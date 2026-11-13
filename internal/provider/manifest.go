package provider

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/prulloac/weave-git/internal/source"
)

const (
	maxManifestBytes = 1 << 20
	maxProjects      = 128
	maxDependencies  = 2000
)

var (
	projectRootDirs = map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "target": true,
		"dist": true, "build": true, ".venv": true,
	}
	manifestPriority = []manifestKind{
		{Name: "go.mod", Ecosystem: "go"},
		{Name: "package.json", Ecosystem: "npm"},
		{Name: "Cargo.toml", Ecosystem: "cargo"},
		{Name: "pyproject.toml", Ecosystem: "python"},
	}
	cargoVersionPattern = regexp.MustCompile(`\bversion\s*=\s*"([^"]*)"`)
)

type manifestKind struct {
	Name      string
	Ecosystem string
}

type projectManifest struct {
	Path         string
	Name         string
	Description  string
	Manifest     string
	Ecosystem    string
	Dependencies []dependencyManifest
}

type dependencyManifest struct {
	Name    string
	Version string
	Scope   string
}

func discoverProjects(root string) ([]projectManifest, []string) {
	warnings := make([]string, 0)
	candidates := map[string]manifestKind{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skip project scan %s: %v", path, err))
			return nil
		}
		if entry.IsDir() {
			if path != root && projectRootDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		kind, ok := manifestFor(entry.Name())
		if !ok {
			return nil
		}
		dir := filepath.Dir(path)
		current, exists := candidates[dir]
		if !exists || priority(kind) < priority(current) {
			if exists {
				warnings = append(warnings, fmt.Sprintf("multiple manifests in %s; using %s", relativeProjectPath(root, dir), kind.Name))
			}
			candidates[dir] = kind
		} else {
			warnings = append(warnings, fmt.Sprintf("multiple manifests in %s; using %s", relativeProjectPath(root, dir), current.Name))
		}
		return nil
	})
	if err != nil {
		return nil, append(warnings, err.Error())
	}
	dirs := make([]string, 0, len(candidates))
	for dir := range candidates {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	if len(dirs) > maxProjects {
		warnings = append(warnings, fmt.Sprintf("project limit reached; %d manifests skipped", len(dirs)-maxProjects))
		dirs = dirs[:maxProjects]
	}
	projects := make([]projectManifest, 0, len(dirs))
	remaining := maxDependencies
	for _, dir := range dirs {
		kind := candidates[dir]
		path := filepath.Join(dir, kind.Name)
		info, err := os.Stat(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skip manifest %s: %v", relativeProjectPath(root, path), err))
			continue
		}
		if info.Size() > maxManifestBytes {
			warnings = append(warnings, fmt.Sprintf("skip manifest %s: exceeds %d bytes", relativeProjectPath(root, path), maxManifestBytes))
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skip manifest %s: %v", relativeProjectPath(root, path), err))
			continue
		}
		project, projectWarnings := parseManifest(kind, data)
		warnings = append(warnings, projectWarnings...)
		project.Path = relativeProjectPath(root, dir)
		project.Manifest = relativeProjectPath(root, path)
		project.Ecosystem = kind.Ecosystem
		if project.Name == "" {
			project.Name = filepath.Base(root)
			if project.Path != "" {
				project.Name = filepath.Base(dir)
			}
		}
		if len(project.Dependencies) > remaining {
			project.Dependencies = project.Dependencies[:remaining]
			warnings = append(warnings, "dependency limit reached; remaining dependencies skipped")
			remaining = 0
		} else {
			remaining -= len(project.Dependencies)
		}
		projects = append(projects, project)
	}
	return projects, warnings
}

func manifestFor(name string) (manifestKind, bool) {
	for _, kind := range manifestPriority {
		if name == kind.Name {
			return kind, true
		}
	}
	return manifestKind{}, false
}

func priority(kind manifestKind) int {
	for index, value := range manifestPriority {
		if value.Name == kind.Name {
			return index
		}
	}
	return len(manifestPriority)
}

func relativeProjectPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." {
		return ""
	}
	return filepath.ToSlash(relative)
}

func parseManifest(kind manifestKind, data []byte) (projectManifest, []string) {
	switch kind.Ecosystem {
	case "go":
		return parseGoMod(string(data)), nil
	case "npm":
		return parsePackageJSON(data)
	case "cargo":
		return parseCargoTOML(string(data))
	case "python":
		return parsePyProject(string(data))
	default:
		return projectManifest{}, []string{"unsupported manifest " + kind.Name}
	}
}

func parseGoMod(body string) projectManifest {
	project := projectManifest{}
	dependencies := make([]dependencyManifest, 0)
	inRequire := false
	for _, line := range strings.Split(body, "\n") {
		indirect := strings.Contains(line, "// indirect")
		trimmed := strings.TrimSpace(line)
		if index := strings.Index(trimmed, "//"); index >= 0 {
			trimmed = strings.TrimSpace(trimmed[:index])
		}
		if trimmed == "" {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "module "):
			project.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "module "))
		case trimmed == "require (":
			inRequire = true
		case inRequire && trimmed == ")":
			inRequire = false
		case inRequire:
			if dependency, ok := goModDependency(trimmed, indirect); ok {
				dependencies = append(dependencies, dependency)
			}
		case strings.HasPrefix(trimmed, "require "):
			if dependency, ok := goModDependency(strings.TrimSpace(strings.TrimPrefix(trimmed, "require ")), indirect); ok {
				dependencies = append(dependencies, dependency)
			}
		}
	}
	project.Dependencies = sortDependencies(dependencies)
	return project
}

func goModDependency(line string, indirect bool) (dependencyManifest, bool) {
	fields := strings.Fields(line)
	if len(fields) < 1 {
		return dependencyManifest{}, false
	}
	dependency := dependencyManifest{Name: fields[0], Scope: "runtime"}
	if len(fields) > 1 {
		dependency.Version = fields[1]
	}
	if indirect {
		dependency.Scope = "indirect"
	}
	return dependency, true
}

func parsePackageJSON(data []byte) (projectManifest, []string) {
	var value struct {
		Name                 string            `json:"name"`
		Description          string            `json:"description"`
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return projectManifest{}, []string{fmt.Sprintf("invalid package.json: %v", err)}
	}
	dependencies := make([]dependencyManifest, 0)
	appendMap := func(values map[string]string, scope string) {
		for name, version := range values {
			dependencies = append(dependencies, dependencyManifest{Name: name, Version: version, Scope: scope})
		}
	}
	appendMap(value.Dependencies, "runtime")
	appendMap(value.DevDependencies, "development")
	appendMap(value.PeerDependencies, "peer")
	appendMap(value.OptionalDependencies, "optional")
	return projectManifest{Name: value.Name, Description: value.Description, Dependencies: sortDependencies(dependencies)}, nil
}

func parseCargoTOML(body string) (projectManifest, []string) {
	sections := parseTOMLSections(body)
	project := sections["package"]
	result := projectManifest{
		Name:        tomlString(project["name"]),
		Description: tomlString(project["description"]),
	}
	dependencies := make([]dependencyManifest, 0)
	for _, section := range []struct {
		Name  string
		Scope string
	}{
		{"dependencies", "runtime"},
		{"dev-dependencies", "development"},
		{"build-dependencies", "build"},
	} {
		values := sections[section.Name]
		names := make([]string, 0, len(values))
		for name := range values {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			dependencies = append(dependencies, dependencyManifest{
				Name:    name,
				Version: tomlDependencyVersion(values[name]),
				Scope:   section.Scope,
			})
		}
	}
	result.Dependencies = sortDependencies(dependencies)
	return result, nil
}

func parsePyProject(body string) (projectManifest, []string) {
	sections := parseTOMLSections(body)
	project := sections["project"]
	result := projectManifest{
		Name:        tomlString(project["name"]),
		Description: tomlString(project["description"]),
	}
	dependencies := make([]dependencyManifest, 0)
	for _, value := range tomlArray(project["dependencies"]) {
		if dependency, ok := pythonDependency(value, "runtime"); ok {
			dependencies = append(dependencies, dependency)
		}
	}
	optional := sections["project.optional-dependencies"]
	names := make([]string, 0, len(optional))
	for name := range optional {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range tomlArray(optional[name]) {
			if dependency, ok := pythonDependency(value, "optional"); ok {
				dependencies = append(dependencies, dependency)
			}
		}
	}
	result.Dependencies = sortDependencies(dependencies)
	return result, nil
}

func pythonDependency(value, scope string) (dependencyManifest, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return dependencyManifest{}, false
	}
	nameEnd := len(value)
	for index, char := range value {
		if char == '[' || char == '<' || char == '>' || char == '=' || char == '!' || char == '~' || char == ';' || char == ' ' {
			nameEnd = index
			break
		}
	}
	name := strings.TrimSpace(value[:nameEnd])
	version := strings.TrimSpace(value[nameEnd:])
	if strings.HasPrefix(version, "[") {
		if end := strings.Index(version, "]"); end >= 0 {
			version = strings.TrimSpace(version[end+1:])
		}
	}
	if strings.HasPrefix(version, ";") {
		version = ""
	}
	if name == "" {
		return dependencyManifest{}, false
	}
	return dependencyManifest{Name: name, Version: version, Scope: scope}, true
}

func sortDependencies(values []dependencyManifest) []dependencyManifest {
	seen := make(map[string]bool, len(values))
	result := make([]dependencyManifest, 0, len(values))
	for _, value := range values {
		key := value.Name + "\x00" + value.Scope + "\x00" + value.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		if result[i].Scope != result[j].Scope {
			return result[i].Scope < result[j].Scope
		}
		return result[i].Version < result[j].Version
	})
	return result
}

func parseTOMLSections(body string) map[string]map[string]string {
	sections := map[string]map[string]string{}
	current := ""
	lines := strings.Split(body, "\n")
	for index := 0; index < len(lines); index++ {
		line := stripTOMLComment(lines[index])
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") && !strings.Contains(trimmed, "=") {
			current = strings.TrimSpace(strings.Trim(trimmed, "[]"))
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = tomlString(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "[") {
			depth := tomlBracketDepth(value)
			for depth > 0 && index+1 < len(lines) {
				index++
				next := stripTOMLComment(lines[index])
				value += " " + strings.TrimSpace(next)
				depth += tomlBracketDepth(next)
			}
		}
		if sections[current] == nil {
			sections[current] = map[string]string{}
		}
		sections[current][key] = value
	}
	return sections
}

func stripTOMLComment(line string) string {
	quote := rune(0)
	for index, char := range line {
		switch {
		case quote != 0 && char == quote:
			quote = 0
		case quote == 0 && (char == '"' || char == '\''):
			quote = char
		case quote == 0 && char == '#':
			return line[:index]
		}
	}
	return line
}

func tomlBracketDepth(value string) int {
	depth := 0
	quote := rune(0)
	for _, char := range value {
		switch {
		case quote != 0 && char == quote:
			quote = 0
		case quote == 0 && (char == '"' || char == '\''):
			quote = char
		case quote == 0 && char == '[':
			depth++
		case quote == 0 && char == ']':
			depth--
		}
	}
	return depth
}

func tomlString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) >= 2 && value[0] == '"' {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	return value
}

func tomlArray(value string) []string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") {
		return nil
	}
	result := make([]string, 0)
	quote := rune(0)
	start := -1
	for index, char := range value {
		switch {
		case quote != 0 && char == quote:
			if start >= 0 {
				result = append(result, tomlString(value[start:index+1]))
				start = -1
			}
			quote = 0
		case quote == 0 && (char == '"' || char == '\''):
			quote = char
			start = index
		}
	}
	return result
}

func tomlDependencyVersion(value string) string {
	value = strings.TrimSpace(value)
	if match := cargoVersionPattern.FindStringSubmatch(value); len(match) == 2 {
		return match[1]
	}
	return tomlString(value)
}

func toSourceProjects(projects []projectManifest) []source.Project {
	result := make([]source.Project, 0, len(projects))
	for _, project := range projects {
		dependencies := make([]source.Dependency, 0, len(project.Dependencies))
		for _, dependency := range project.Dependencies {
			dependencies = append(dependencies, source.Dependency{
				Name: dependency.Name, Version: dependency.Version, Scope: dependency.Scope,
			})
		}
		result = append(result, source.Project{
			Path:         project.Path,
			Name:         project.Name,
			Description:  project.Description,
			Manifest:     project.Manifest,
			Ecosystem:    project.Ecosystem,
			Dependencies: dependencies,
		})
	}
	return result
}
