# AGENTS.md

`weave-git` is the first-party source-provider extension for Weave. It resolves git repositories
into commit-pinned local snapshots and speaks `weave.source/v1` over stdin/stdout. It never imports
Weave packages and never parses source code.

## Commands

- Build: `go build -o weave-git .`
- All tests: `go test ./...`
- Race: `go test -race -count=1 -shuffle=on ./...`
- Vet / format: `go vet ./...`; `gofmt -l .` must print nothing
- Describe: `go run . --describe --json`
- Version: `go run . --version`
- Cache GC: `go run . gc --cache-dir <dir> [--keep <source-id> ...] [--dry-run] [--json]`
- Full gate: `./scripts/verify.sh`
- User-level install: `./scripts/install.sh --help` (or `scripts/install.ps1` on Windows)

## Layout

```text
main.go                 CLI, describe/version and source request entrypoint
internal/source/        weave.source/v1 wire types
internal/provider/      local and remote git resolution, bare mirrors and snapshot materialization
docs/bdd/               extension-side behavior scenarios
scripts/                verification
```

## Invariants

1. **Static builds:** no C libraries, no CGO compiler and no language server.
2. **Contract only:** speak `weave.source/v1`; never import Weave packages or read its database.
3. **Acquisition only:** resolve refs and materialize snapshots. Do not parse code, symbols or
   imports; that belongs to analyzers such as `weave-code`.
4. **Stable identity:** honor the `source_id` supplied by core. The ref and commit never define the
   worktree path.
5. **Source only:** never write into the source repository or the bundle.
6. **No execution:** never run repository code, build tools or hooks.
7. **Conservative network:** only remote URIs explicitly supplied by core are fetched. Store and MCP
   reads never fetch, and the local path slice performs no network access.
8. **Deterministic:** the same commit resolves to the same worktree contents and provenance.
9. **Bounded:** enforce the file, project, time and symlink limits from the specification.

## Testing quirks

- Fixtures create temporary git repositories with the real `git` CLI; tests skip when git is absent.
- Remote tests use `git+file://` bare repositories, so they exercise mirror fetch and offline reuse
  without network access.
- Submodule tests use `protocol.file.allow=always` for local fixtures.
- Snapshot tests assert that `.git` is not copied into the worktree.
- The provider never needs Weave or `weave-code` to build or test.

## Documentation split

- This repository owns usage docs and extension-side behavior.
- The behavioral contract lives in
  [`weave-docs/specs/weave-git.md`](https://github.com/prulloac/weave-docs/blob/main/specs/weave-git.md).
- Core source resolution, schema and MCP behavior live in the Weave repository.

## Release

- Tags use `vX.Y.Z`; the release workflow builds six static archives and a draft GitHub Release.
- Keep the binary name `weave-git`; Weave discovers extensions by the `weave-` prefix.
- Installers default to a user-writable directory and never require administrator rights.
- Release archives include the binary, `README.md` and `LICENSE`; checksums are published.
