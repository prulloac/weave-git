# Weave Git Source Provider

`weave-git` resolves local git repositories into commit-pinned snapshots for Weave. It is the first
source-provider extension and speaks `weave.source/v1`.

```sh
git+https://host/team/repo.git
    -> resolve commit
    -> materialize a read-only worktree under WEAVE_CACHE_DIR
    -> hand the snapshot to Weave analyzers
```

## Status

This first slice supports **local repositories**. Remote fetch, workspace object sources, installers
and release packaging follow in later slices.

## Supported requests

```json
{
  "v": "weave.source/v1",
  "type": "source_request",
  "source_id": "src-8f3c2a1d4b5e6f7",
  "uri": "/home/user/src/weave",
  "ref": "HEAD",
  "cache_dir": "/home/user/.cache/weave",
  "options": {"worktree": false}
}
```

The provider returns the stable worktree path, full commit, ref, repository name, README path, title
and description.

## Behavior

- `worktree: false` materializes a committed tree under
  `$WEAVE_CACHE_DIR/sources/<source_id>/worktree`.
- `worktree: true` returns the local checkout and records the current `HEAD` plus a dirty warning.
- Snapshot worktrees contain no `.git` directory.
- Re-ingesting the same commit reuses the worktree.
- A ref or commit change replaces the worktree contents under the same path.
- Missing READMEs produce a warning and an empty repository body.
- Unsafe symlinks are skipped with a warning.
- Remote git URIs are rejected until the remote slice lands.

## Develop

```sh
go build -o weave-git .
go test ./...
go vet ./...
./scripts/verify.sh
```

Contract:
[`weave-git.md`](https://github.com/prulloac/weave-docs/blob/main/specs/weave-git.md).

## Issues

This repository does not accept issues. Report first-party defects in
[`prulloac/weave`](https://github.com/prulloac/weave/issues/new/choose).

## License

MIT.
