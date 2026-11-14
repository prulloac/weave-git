#!/usr/bin/env bash
set -euo pipefail

# Cross-repository smoke test: build real weave and weave-git binaries, ingest a
# local git source through the provider contract and verify the result.

cd "$(dirname "$0")/.."
repo_root="$(pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

weave_src="${WEAVE_SOURCE:-}"
if [ -z "$weave_src" ]; then
  weave_ref="${WEAVE_REF:-main}"
  git clone --quiet --depth 1 --branch "$weave_ref" https://github.com/prulloac/weave.git "$work/weave-src"
  weave_src="$work/weave-src"
fi

mkdir -p "$work/bin"
(cd "$weave_src" && go build -o "$work/bin/weave" ./cmd/weave)
go build -o "$work/bin/weave-git" .
export PATH="$work/bin:$PATH"
export WEAVE_CACHE_DIR="$work/cache"

mkdir -p "$work/repo"
(
  cd "$work/repo"
  git init -q
  git config user.email "integration@weave.local"
  git config user.name "Weave Integration"
  printf '# Integration Fixture\n\nA repository ingested by real binaries.\n' > README.md
  printf 'alpha\n' > notes.md
  printf '{"name":"integration-fixture","description":"Integration fixture","dependencies":{"react":"^18"}}\n' > package.json
  git add .
  git commit -q -m initial
  branch="$(git symbolic-ref --short HEAD)"
  git clone --bare --quiet "$work/repo" "$work/bare.git"
  printf '%s\n' "$branch" > "$work/branch"
)
branch="$(cat "$work/branch")"

result="$("$work/bin/weave" ingest "git+file://$work/bare.git" --git --ref "$branch" --history --history-limit 5 --db "$work/index.db" --json)"
printf '%s\n' "$result"
printf '%s' "$result" | grep -q '"ok":true'

workspace="$work/workspace"
"$work/bin/weave" workspace init "$workspace" --json >/dev/null
"$work/bin/weave" source add core "git+file://$work/bare.git" --git --ref "$branch" --history --history-limit 5 --workspace "$workspace" --json >/dev/null
"$work/bin/weave" ingest --workspace "$workspace" --json >/dev/null
sources="$("$work/bin/weave" source list --workspace "$workspace" --json)"
printf '%s\n' "$sources"
printf '%s' "$sources" | grep -q '"provider":"git"'
printf '%s' "$sources" | grep -q '"commit":"'
snapshot="$(printf '%s' "$sources" | sed -n 's/.*"path":"\([^"]*\)".*/\1/p')"
if [ -n "$snapshot" ]; then
  dependencies="$("$work/bin/weave" list "$snapshot" --type ExternalPackage --db "$workspace/.weave/index.db" --json)"
  printf '%s\n' "$dependencies"
  printf '%s' "$dependencies" | grep -q '"_package/npm/react"'
  commits="$("$work/bin/weave" list "$snapshot" --type Commit --db "$workspace/.weave/index.db" --json)"
  printf '%s\n' "$commits"
  printf '%s' "$commits" | grep -q '"_commit/'
  graph="$("$work/bin/weave" graph "$snapshot" --relation depends_on --db "$workspace/.weave/index.db" --json)"
  printf '%s\n' "$graph"
  printf '%s' "$graph" | grep -q '"_package/npm/react"'
fi

echo "integration: ok"
