package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/prulloac/weave-git/internal/source"
)

// exportCheckout materializes an opt-in checkout so submodules and Git LFS objects are included.
func exportCheckout(root, commit, stage string, options source.Options) ([]string, error) {
	warnings := make([]string, 0)
	if _, err := gitBytes("", "clone", "--quiet", "--no-checkout", root, stage); err != nil {
		return warnings, err
	}
	args := []string{"-c", "core.hooksPath=" + os.DevNull}
	if !options.LFS {
		args = append(args, "-c", "filter.lfs.smudge=cat", "-c", "filter.lfs.process=", "-c", "filter.lfs.required=false")
	}
	args = append(args, "checkout", "--quiet", "--detach", commit)
	if _, err := gitBytes(stage, args...); err != nil {
		return warnings, err
	}
	if options.Submodules {
		submoduleArgs := []string{
			"-c", "core.hooksPath=" + os.DevNull,
			"-c", "protocol.file.allow=always",
			"submodule", "update", "--init", "--recursive",
		}
		if _, err := gitBytes(stage, submoduleArgs...); err != nil {
			return warnings, err
		}
	}
	if options.LFS {
		if _, err := exec.LookPath("git-lfs"); err != nil {
			warnings = append(warnings, "git-lfs is not installed; Git LFS pointers were not expanded")
		} else {
			lfsArgs := []string{"lfs", "pull"}
			if _, err := gitBytes(stage, lfsArgs...); err != nil {
				return warnings, err
			}
			if options.Submodules {
				foreach := []string{"submodule", "foreach", "--recursive", "git lfs pull"}
				if _, err := gitBytes(stage, foreach...); err != nil {
					return warnings, err
				}
			}
		}
	}
	if err := stripGitMetadata(stage); err != nil {
		return warnings, err
	}
	symlinkWarnings, err := sanitizeSymlinks(stage)
	warnings = append(warnings, symlinkWarnings...)
	if err != nil {
		return warnings, err
	}
	if err := validateTree(stage, defaultLimits); err != nil {
		return warnings, err
	}
	return warnings, nil
}

func stripGitMetadata(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || entry.Name() != ".git" {
			return nil
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}

func sanitizeSymlinks(root string) ([]string, error) {
	warnings := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if filepath.IsAbs(target) || escapes(root, path, target) {
			relative, _ := filepath.Rel(root, path)
			if err := os.Remove(path); err != nil {
				return err
			}
			warnings = append(warnings, "skipped unsafe symlink "+filepath.ToSlash(relative))
		}
		return nil
	})
	return warnings, err
}

func validateTree(root string, limits snapshotLimits) error {
	files := 0
	total := int64(0)
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		files++
		if files > limits.MaxFiles {
			return fmt.Errorf("snapshot exceeds %d files", limits.MaxFiles)
		}
		if info.Size() > limits.MaxFileBytes {
			relative, _ := filepath.Rel(root, path)
			return fmt.Errorf("snapshot file %q is %d bytes, limit is %d", filepath.ToSlash(relative), info.Size(), limits.MaxFileBytes)
		}
		total += info.Size()
		if total > limits.MaxProjectBytes {
			return fmt.Errorf("snapshot exceeds %d bytes", limits.MaxProjectBytes)
		}
		return nil
	})
}

func usesCheckout(options source.Options) bool {
	return options.Submodules || options.LFS
}
