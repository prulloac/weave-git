# Weave Git Source Provider

`weave-git` resolves local and remote git repositories into commit-pinned snapshots for Weave. It is
the first source-provider extension and speaks `weave.source/v1`.

```sh
git+https://host/team/repo.git
    -> resolve commit
    -> materialize a read-only worktree under WEAVE_CACHE_DIR
    -> hand the snapshot to Weave analyzers
```

## Status

This slice supports **local and remote repositories**. Remote fetch uses the operator's system
`git`, stores a bare mirror under `$WEAVE_CACHE_DIR/git`, and reuses cached commits when the remote
is unavailable in permissive mode. Workspace object sources, installers and release packaging follow
in later slices.

## Supported requests

```json
{
  "v": "weave.source/v1",
  "type": "source_request",
  "source_id": "src-8f3c2a1d4b5e6f7",
  "uri": "/home/user/src/weave",
  "ref": "HEAD",
  "cache_dir": "/home/user/.cache/weave",
  "options": {"worktree": false, "depth": 1, "permissive": false}
}
```

The provider returns the stable worktree path, full commit, ref, repository name, README path, title
and description.

## Behavior

- `worktree: false` materializes a committed tree under
  `$WEAVE_CACHE_DIR/sources/<source_id>/worktree`.
- `worktree: true` returns the local checkout and records the current `HEAD` plus a dirty warning.
  Worktree mode is local-only.
- Remote URIs (`git+https`, `git+ssh`, SCP-like and `git+file`) are fetched into a bare mirror under
  `$WEAVE_CACHE_DIR/git/<host>/<owner>/<repo>.git`.
- `options.depth` passes a shallow-fetch hint to the mirror.
- `options.submodules` materializes committed submodule trees; `options.lfs` expands Git LFS objects
  through `git lfs pull`. Both default to off and use a temporary checkout under the cache.
- Snapshot worktrees contain no `.git` directory, including submodule metadata.
- Re-ingesting the same commit reuses the worktree.
- A ref or commit change replaces the worktree contents under the same path.
- If a remote is unreachable and the request is permissive, the cached commit for the same ref is
  reused with a warning; strict requests fail.
- Missing READMEs produce a warning and an empty repository body.
- Unsafe symlinks are skipped with a warning.
- Snapshots enforce 4 MiB per file, 512 MiB per project and 10,000 files.
- Credentials are read from the operator's git configuration or SSH agent and never persisted.

## Cache maintenance

```sh
weave-git gc --cache-dir "$WEAVE_CACHE_DIR" --json
weave-git gc --cache-dir "$WEAVE_CACHE_DIR" --keep src-8f3c2a1d4b5e6f7 --dry-run
```

`gc` removes stale `worktree.stage-*` and `worktree.old` trees, removes source caches without a valid
`state.json` or worktree, and runs `git gc --prune=now` on bare mirrors. Mirrors are never deleted.
When one or more `--keep` IDs are given, that set is authoritative: other source caches are removed.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/prulloac/weave-git/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/prulloac/weave-git/main/scripts/install.ps1 | iex
```

Installers default to a user-writable directory and never require administrator rights.

## Develop

```sh
go build -o weave-git .
go test ./...
go vet ./...
./scripts/verify.sh
WEAVE_SOURCE=../weave ./scripts/integration.sh
```

Contract:
[`weave-git.md`](https://github.com/prulloac/weave-docs/blob/main/specs/weave-git.md).

## Issues

This repository does not accept issues. Report first-party defects in
[`prulloac/weave`](https://github.com/prulloac/weave/issues/new/choose).

## License

MIT.
