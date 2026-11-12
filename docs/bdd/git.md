---
type: behavior
title: "Weave Git Provider Behavior"
description: "Extension-side scenarios for local git snapshot materialization, ref resolution, stable worktrees and repository metadata."
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

### Scenario: Remote sources are deferred

- **Given** a `git+https://` source URI
- **When** the local provider slice handles it
- **Then** the request fails clearly
- **And** no network operation is attempted

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
