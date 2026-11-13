package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GCOptions controls one cache cleanup pass.
type GCOptions struct {
	CacheDir string
	Keep     []string
	DryRun   bool
}

// GCResult reports cache cleanup actions.
type GCResult struct {
	Removed  []string `json:"removed,omitempty"`
	Pruned   []string `json:"pruned,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// CollectGarbage removes stale staging trees and incomplete source caches.
// When Keep is non-empty, it is authoritative: source directories not listed are removed.
// Mirrors are retained and compacted with git gc.
func CollectGarbage(options GCOptions) (GCResult, error) {
	cacheDir := strings.TrimSpace(options.CacheDir)
	if cacheDir == "" {
		return GCResult{}, fmt.Errorf("cache_dir is required")
	}
	absolute, err := filepath.Abs(cacheDir)
	if err != nil {
		return GCResult{}, err
	}
	if info, err := os.Stat(absolute); err != nil || !info.IsDir() {
		return GCResult{}, nil
	}
	keep := make(map[string]bool, len(options.Keep))
	for _, id := range options.Keep {
		if value := strings.TrimSpace(id); value != "" {
			keep[value] = true
		}
	}
	result := GCResult{}
	sourcesDir := filepath.Join(absolute, "sources")
	entries, err := os.ReadDir(sourcesDir)
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		base := filepath.Join(sourcesDir, entry.Name())
		result.Removed = append(result.Removed, removeStale(base, absolute, options.DryRun)...)
		remove := false
		if len(keep) > 0 && !keep[entry.Name()] {
			remove = true
		} else if _, ok := readState(base); !ok || !worktreeExists(base) {
			remove = true
		}
		if !remove {
			continue
		}
		if !options.DryRun {
			if err := os.RemoveAll(base); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("remove %s: %v", entry.Name(), err))
				continue
			}
		}
		result.Removed = append(result.Removed, relativePath(absolute, base))
	}
	if err := collectMirrors(absolute, options.DryRun, &result); err != nil {
		return result, err
	}
	sort.Strings(result.Removed)
	sort.Strings(result.Pruned)
	sort.Strings(result.Warnings)
	return result, nil
}

func removeStale(base, cacheDir string, dryRun bool) []string {
	removed := make([]string, 0)
	entries, err := os.ReadDir(base)
	if err != nil {
		return removed
	}
	for _, entry := range entries {
		name := entry.Name()
		if name != "worktree.old" && !strings.HasPrefix(name, "worktree.stage-") {
			continue
		}
		target := filepath.Join(base, name)
		if !dryRun {
			if err := os.RemoveAll(target); err != nil {
				continue
			}
		}
		removed = append(removed, relativePath(cacheDir, target))
	}
	return removed
}

func collectMirrors(cacheDir string, dryRun bool, result *GCResult) error {
	root := filepath.Join(cacheDir, "git")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("walk %s: %v", relativePath(cacheDir, path), err))
			return nil
		}
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".git") {
			return nil
		}
		if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
			return nil
		}
		relative := relativePath(cacheDir, path)
		if !dryRun {
			if _, err := gitBytes(path, "gc", "--quiet", "--prune=now"); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("gc %s: %v", relative, err))
				return nil
			}
		}
		result.Pruned = append(result.Pruned, relative)
		return filepath.SkipDir
	})
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
