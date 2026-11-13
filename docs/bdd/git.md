---
type: behavior
title: "Weave Git Provider Behavior"
description: "Extension-side scenarios for local and remote git snapshot materialization, ref resolution, stable worktrees, offline cache reuse and repository metadata."
status: draft
---

# Behavior Scenarios: Weave Git Provider

Core behavior:
[weave-git.md](https://github.com/prulloac/weave/blob/main/docs/bdd/weave-git.md)

Specification:
[weave-git.md](https://github.com/prulloac/weave-docs/blob/main/specs/weave-git.md)

Go tests: `internal/provider/provider_test.go`, `main_test.go`

## Feature: Local snapshot

### Scenario: A local repository becomes a worktree

- **Given** a local git repository with a README and source files
- **When** the provider receives a source request
- **Then** it resolves the commit
- **And** it materializes a worktree under the requested cache directory
- **And** the worktree contains no `.git` directory

### Scenario: The source identity is stable

- **Given** the same `source_id`
- **When** the ref moves to a new commit
- **Then** the worktree path stays the same
- **And** the returned commit changes

### Scenario: The same commit is reused

- **Given** a provider cache with the requested commit
- **When** the same source and commit are requested again
- **Then** the existing worktree is reused
- **And** its contents are not materialized again

### Scenario: A ref change replaces the worktree

- **Given** a source materialized at `main`
- **When** the same source is requested at a tag or commit
- **Then** the same worktree path is updated
- **And** the returned ref and commit describe the new snapshot

### Scenario: Worktree mode is local-only

- **Given** a local repository with uncommitted changes
- **When** the request sets `worktree: true`
- **Then** the provider returns the local checkout
- **And** it warns that the worktree is dirty

## Feature: Remote sources

### Scenario: A remote repository becomes a commit-pinned snapshot

- **Given** a remote git URI in HTTPS, SSH, SCP-like or `file://` form
- **When** the provider receives a source request
- **Then** it fetches into a bare mirror under `$WEAVE_CACHE_DIR/git`
- **And** it resolves the requested ref to a full commit
- **And** it materializes the stable worktree from that commit

### Scenario: Remote worktree mode is invalid

- **Given** a remote git URI
- **When** the request sets `worktree: true`
- **Then** the provider fails clearly
- **And** it does not return the mirror or cache as a mutable checkout

### Scenario: An offline remote reuses the cached commit

- **Given** a remote source that was materialized before
- **When** the remote is unreachable
- **And** the request is permissive
- **Then** the provider reuses the cached commit
- **And** it returns a warning naming the cached commit
- **And** the worktree path does not change

### Scenario: A strict offline remote fails

- **Given** a remote source that was materialized before
- **When** the remote is unreachable
- **And** the request is strict
- **Then** the provider fails
- **And** it does not silently fall back to another ref

### Scenario: Credentials are never echoed

- **Given** a remote URI containing a password or token
- **When** a fetch fails
- **Then** the error redacts the userinfo
- **And** the URI written to provenance contains no password

## Feature: Optional checkout content

### Scenario: Submodules are skipped by default

- **Given** a repository with a committed submodule
- **When** the source is materialized without `submodules`
- **Then** the submodule working tree is not populated

### Scenario: Submodules can be included

- **Given** a repository with a committed submodule
- **When** the request sets `submodules: true`
- **Then** the submodule tree is materialized
- **And** no `.git` file or directory remains in the worktree

### Scenario: LFS requires the operator tool

- **Given** a repository using Git LFS
- **When** the request sets `lfs: true`
- **Then** the provider runs `git lfs pull` when `git-lfs` is installed
- **And** it warns and leaves pointers when `git-lfs` is absent

### Scenario: LFS filters stay disabled by default

- **Given** a repository using Git LFS
- **When** the request does not set `lfs`
- **Then** archive materialization leaves pointer files
- **And** no LFS network fetch is attempted

## Feature: Projects and dependencies

### Scenario: A root manifest defines the repository project

- **Given** a repository with `go.mod`, `package.json`, `Cargo.toml` or `pyproject.toml` at the root
- **When** the source is materialized
- **Then** the response contains one project for the root
- **And** its name, description and dependencies come from the manifest

### Scenario: Nested manifests define monorepo projects

- **Given** a repository with manifests in nested directories
- **When** the source is materialized
- **Then** each directory with a manifest becomes one project
- **And** project paths are bundle-relative and deterministic

### Scenario: Dependencies retain versions and scopes

- **Given** manifests with runtime, development, build, peer, optional and indirect dependencies
- **When** the source is materialized
- **Then** each dependency carries its name, version constraint and scope

### Scenario: Manifest discovery is bounded

- **Given** a repository with manifests inside `node_modules`, `vendor`, `target`, `dist`, `build` or `.venv`
- **When** the source is materialized
- **Then** those directories are skipped
- **And** manifests over 1 MiB or beyond the project and dependency limits are skipped with a warning

### Scenario: Manifest description falls back when README has none

- **Given** a repository with a manifest description and a README without one
- **When** the source is materialized
- **Then** the repository description comes from the root manifest

## Feature: Repository metadata

### Scenario: README title and description

- **Given** a README with a heading and a paragraph
- **When** the source is materialized
- **Then** the repository title comes from the heading
- **And** the description comes from the first paragraph

### Scenario: Missing README

- **Given** a repository without a root README
- **When** the source is materialized
- **Then** the repository name falls back to the directory name
- **And** one warning reports the missing README

## Feature: Safety

### Scenario: Unsafe symlinks are skipped

- **Given** a commit containing a symlink that escapes the worktree
- **When** the source is materialized
- **Then** the symlink is skipped
- **And** a warning is returned

### Scenario: Unsafe archive paths are rejected

- **Given** a git archive entry with an absolute or parent-escaping path
- **When** the source is materialized
- **Then** extraction fails
- **And** no out-of-root file is written

### Scenario: Snapshot limits are enforced

- **Given** a repository whose file, project size or file count exceeds the limits
- **When** the source is materialized
- **Then** extraction fails before the worktree swap
- **And** the previous snapshot remains valid

## Feature: Cache maintenance

### Scenario: Garbage collection removes stale staging trees

- **Given** a source cache with `worktree.stage-*` or `worktree.old` directories
- **When** `weave-git gc` runs
- **Then** the staging and old trees are removed
- **And** the active `worktree` is retained

### Scenario: Garbage collection removes incomplete sources

- **Given** a source directory without `state.json` or without a worktree
- **When** `weave-git gc` runs
- **Then** the incomplete directory is removed

### Scenario: Garbage collection keeps an authoritative set

- **Given** `weave-git gc --keep <source-id>`
- **When** other source directories exist
- **Then** only the listed source IDs and active mirrors are retained

### Scenario: Garbage collection is dry-runnable

- **Given** `weave-git gc --dry-run`
- **When** stale entries exist
- **Then** the report lists them
- **And** nothing is deleted

### Scenario: Mirror objects are compacted

- **Given** a bare mirror under the cache
- **When** `weave-git gc` runs
- **Then** git object pruning runs on the mirror
- **And** the mirror directory is retained
