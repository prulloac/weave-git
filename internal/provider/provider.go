// Package provider materializes local git repositories into stable worktrees.
package provider

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/prulloac/weave-git/internal/source"
)

var (
	sourceIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	headingPattern  = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
)

// defaultLimits mirror the code-extension snapshot limits.
var defaultLimits = snapshotLimits{
	MaxFileBytes:    4 << 20,
	MaxProjectBytes: 512 << 20,
	MaxFiles:        10000,
}

type snapshotLimits struct {
	MaxFileBytes    int64
	MaxProjectBytes int64
	MaxFiles        int
}

type state struct {
	Commit string `json:"commit"`
	Ref    string `json:"ref,omitempty"`
	URI    string `json:"uri,omitempty"`
}

// Materialize resolves a local or remote git source into a read-only worktree.
func Materialize(request source.Request) (source.Response, error) {
	if !sourceIDPattern.MatchString(request.SourceID) {
		return source.Response{}, fmt.Errorf("invalid source_id %q", request.SourceID)
	}
	loc, err := parseLocation(request.URI)
	if err != nil {
		return source.Response{}, err
	}
	request.URI = loc.URI
	if loc.Remote && request.Options.Worktree {
		return source.Response{}, errors.New("worktree mode requires a local repository")
	}
	warnings := make([]string, 0)
	archiveRoot := loc.Path
	var commit string
	if loc.Remote {
		var remoteWarnings []string
		commit, archiveRoot, remoteWarnings, err = resolveRemote(request, loc)
		if err != nil {
			return source.Response{}, err
		}
		warnings = append(warnings, remoteWarnings...)
	} else {
		commit, err = resolveCommit(loc.Path, request.Ref)
		if err != nil {
			return source.Response{}, err
		}
	}
	if request.Options.Worktree {
		repository, repositoryWarnings := describeRepository(loc.Path, loc.Name)
		warnings = append(warnings, repositoryWarnings...)
		dirty, _ := isDirty(loc.Path)
		if dirty {
			warnings = append(warnings, "working tree has uncommitted changes")
		}
		return response(request, loc, loc.Path, commit, false, repository, warnings), nil
	}
	if strings.TrimSpace(request.CacheDir) == "" {
		return source.Response{}, errors.New("cache_dir is required")
	}
	worktree, materializeWarnings, err := materialize(request, archiveRoot, commit, warnings)
	if err != nil {
		return source.Response{}, err
	}
	warnings = materializeWarnings
	repository, repositoryWarnings := describeRepository(worktree, loc.Name)
	warnings = append(warnings, repositoryWarnings...)
	if loc.Remote {
		repository.Host = loc.Host
		repository.Owner = loc.Owner
	}
	return response(request, loc, worktree, commit, true, repository, warnings), nil
}

func resolveCommit(root, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		ref = "HEAD"
	}
	output, err := git(root, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve ref %q: %w", ref, err)
	}
	return strings.TrimSpace(output), nil
}

func materialize(request source.Request, root, commit string, warnings []string) (string, []string, error) {
	base := sourceBase(request)
	worktree := filepath.Join(base, "worktree")
	stage := filepath.Join(base, fmt.Sprintf("worktree.stage-%d", os.Getpid()))
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", warnings, err
	}
	if stateMatches(worktree, commit) {
		return worktree, warnings, nil
	}
	_ = os.RemoveAll(stage)
	data, err := gitBytes(root, "archive", "--format=tar", commit)
	if err != nil {
		return "", warnings, err
	}
	symlinkWarnings, err := extract(data, stage)
	if err != nil {
		_ = os.RemoveAll(stage)
		return "", warnings, err
	}
	warnings = append(warnings, symlinkWarnings...)
	old := worktree + ".old"
	_ = os.RemoveAll(old)
	if _, err := os.Stat(worktree); err == nil {
		if err := os.Rename(worktree, old); err != nil {
			_ = os.RemoveAll(stage)
			return "", warnings, err
		}
	}
	if err := os.Rename(stage, worktree); err != nil {
		_ = os.Rename(old, worktree)
		return "", warnings, err
	}
	_ = os.RemoveAll(old)
	if err := writeState(base, state{Commit: commit, Ref: request.Ref, URI: request.URI}); err != nil {
		return "", warnings, err
	}
	return worktree, warnings, nil
}

func stateMatches(worktree, commit string) bool {
	value, ok := readState(filepath.Dir(worktree))
	if !ok {
		return false
	}
	if value.Commit != commit {
		return false
	}
	info, err := os.Stat(worktree)
	return err == nil && info.IsDir()
}

func writeState(base string, value state) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(base, "state.json"), append(data, '\n'), 0o644)
}

func extract(data []byte, destination string) ([]string, error) {
	return extractWithLimits(data, destination, defaultLimits)
}

func extractWithLimits(data []byte, destination string, limits snapshotLimits) ([]string, error) {
	reader := tar.NewReader(bytes.NewReader(data))
	warnings := make([]string, 0)
	files := 0
	total := int64(0)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return warnings, err
		}
		clean := filepath.Clean(filepath.FromSlash(header.Name))
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return warnings, fmt.Errorf("unsafe archive path %q", header.Name)
		}
		target := filepath.Join(destination, clean)
		if !strings.HasPrefix(target, destination+string(filepath.Separator)) {
			return warnings, fmt.Errorf("unsafe archive path %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return warnings, err
			}
		case tar.TypeReg:
			files++
			if files > limits.MaxFiles {
				return warnings, fmt.Errorf("snapshot exceeds %d files", limits.MaxFiles)
			}
			if header.Size > limits.MaxFileBytes {
				return warnings, fmt.Errorf("snapshot file %q is %d bytes, limit is %d", header.Name, header.Size, limits.MaxFileBytes)
			}
			if total+header.Size > limits.MaxProjectBytes {
				return warnings, fmt.Errorf("snapshot exceeds %d bytes", limits.MaxProjectBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return warnings, err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return warnings, err
			}
			written, copyErr := io.Copy(file, io.LimitReader(reader, limits.MaxFileBytes+1))
			closeErr := file.Close()
			if copyErr != nil {
				return warnings, copyErr
			}
			if closeErr != nil {
				return warnings, closeErr
			}
			if written > limits.MaxFileBytes {
				return warnings, fmt.Errorf("snapshot file %q exceeds %d bytes", header.Name, limits.MaxFileBytes)
			}
			total += written
			if total > limits.MaxProjectBytes {
				return warnings, fmt.Errorf("snapshot exceeds %d bytes", limits.MaxProjectBytes)
			}
		case tar.TypeSymlink:
			linkTarget := header.Linkname
			if filepath.IsAbs(linkTarget) || escapes(destination, target, linkTarget) {
				warnings = append(warnings, "skipped unsafe symlink "+header.Name)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return warnings, err
			}
			if err := os.Symlink(linkTarget, target); err != nil {
				return warnings, err
			}
		}
	}
	return warnings, nil
}

func escapes(root, link, target string) bool {
	resolved := filepath.Clean(filepath.Join(filepath.Dir(link), target))
	relative, err := filepath.Rel(root, resolved)
	return err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative)
}

func describeRepository(root, fallbackName string) (source.Repository, []string) {
	name := fallbackName
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(root)
	}
	repository := source.Repository{ID: "_repo", Name: name, Title: name}
	warnings := make([]string, 0)
	readme, ok := findReadme(root)
	if !ok {
		warnings = append(warnings, "no root README.md found")
		return repository, warnings
	}
	data, err := os.ReadFile(filepath.Join(root, readme))
	if err != nil {
		warnings = append(warnings, "cannot read "+readme)
		return repository, warnings
	}
	body := string(data)
	if title := firstHeading(body); title != "" {
		repository.Title = title
	}
	repository.Description = frontmatterDescription(body)
	if repository.Description == "" {
		repository.Description = firstParagraph(body)
	}
	repository.Readme = readme
	if license := findLicense(root); license != "" {
		repository.License = license
	}
	return repository, warnings
}

func findReadme(root string) (string, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(entry.Name()) {
		case "readme.md", "readme.markdown":
			return entry.Name(), true
		}
	}
	return "", false
}

func findLicense(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(strings.ToUpper(entry.Name()), "LICENSE") {
			return entry.Name()
		}
	}
	return ""
}

func firstHeading(body string) string {
	body = stripFrontmatter(body)
	if match := headingPattern.FindStringSubmatch(body); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func frontmatterDescription(body string) string {
	header, ok := frontmatterBlock(body)
	if !ok {
		return ""
	}
	for _, line := range strings.Split(header, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(strings.ToLower(key)) != "description" {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return ""
}

func stripFrontmatter(body string) string {
	if _, ok := frontmatterBlock(body); !ok {
		return body
	}
	end := strings.Index(body[4:], "\n---")
	if end < 0 {
		return body
	}
	return body[4+end+4:]
}

func frontmatterBlock(body string) (string, bool) {
	if !strings.HasPrefix(body, "---\n") {
		return "", false
	}
	end := strings.Index(body[4:], "\n---")
	if end < 0 {
		return "", false
	}
	return body[4 : 4+end], true
}

func firstParagraph(body string) string {
	body = stripFrontmatter(body)
	inFence := false
	paragraph := make([]string, 0)
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "---") {
			continue
		}
		paragraph = append(paragraph, trimmed)
	}
	text := strings.Join(paragraph, " ")
	if len([]rune(text)) > 240 {
		return string([]rune(text)[:240])
	}
	return text
}

func response(request source.Request, loc location, path, commit string, readOnly bool, repository source.Repository, warnings []string) source.Response {
	return source.Response{
		V:    source.ProtocolVersion,
		Type: "source_response",
		Snapshot: source.Snapshot{
			Path:     path,
			SourceID: request.SourceID,
			Commit:   commit,
			Ref:      request.Ref,
			URI:      loc.URI,
			ReadOnly: readOnly,
		},
		Repository: repository,
		Warnings:   warnings,
	}
}

func isDirty(root string) (bool, error) {
	output, err := git(root, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

func git(root string, args ...string) (string, error) {
	data, err := gitBytes(root, args...)
	return string(data), err
}

func gitBytes(root string, args ...string) ([]byte, error) {
	invocation := args
	if root != "" {
		invocation = append([]string{"-C", root}, args...)
	}
	command := exec.Command("git", invocation...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	data, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			message := redactCredentials(strings.TrimSpace(string(exit.Stderr)))
			return nil, fmt.Errorf("git %s: %s", redactCredentials(strings.Join(invocation, " ")), message)
		}
		return nil, err
	}
	return data, nil
}
