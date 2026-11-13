#!/usr/bin/env bash
set -eu

REPO="prulloac/weave-git"
BINARY="weave-git"

usage() {
  cat <<'EOF'
Install weave-git from GitHub Releases without compiling.

Usage: install.sh [options]

Options:
  --version <tag>    Install a specific release tag (default: latest)
  --bin-dir <dir>    Install directory (default: $XDG_BIN_HOME or ~/.local/bin)
  --no-modify-path   Do not edit your shell profile
  -h, --help         Show this help

Environment:
  WEAVE_GIT_VERSION          Same as --version
  WEAVE_GIT_BIN_DIR          Same as --bin-dir
  WEAVE_GIT_NO_MODIFY_PATH   Set to 1 to skip shell profile changes
  GITHUB_TOKEN               Optional token for the GitHub API rate limit

The installer is user-level by default and never uses sudo.
EOF
}

error() {
  printf 'install: %s\n' "$*" >&2
  exit 1
}

version="${WEAVE_GIT_VERSION:-latest}"
bin_dir="${WEAVE_GIT_BIN_DIR:-}"
modify_path=1
if [ "${WEAVE_GIT_NO_MODIFY_PATH:-}" = "1" ]; then
  modify_path=0
fi

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      [ $# -ge 2 ] || error "--version requires a tag"
      version="$2"
      shift 2
      ;;
    --version=*)
      version="${1#*=}"
      shift
      ;;
    --bin-dir)
      [ $# -ge 2 ] || error "--bin-dir requires a directory"
      bin_dir="$2"
      shift 2
      ;;
    --bin-dir=*)
      bin_dir="${1#*=}"
      shift
      ;;
    --no-modify-path)
      modify_path=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      error "unknown option: $1"
      ;;
  esac
done

if [ -z "$bin_dir" ]; then
  bin_dir="${XDG_BIN_HOME:-${HOME}/.local/bin}"
fi

case "$(uname -s)" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  MINGW*|MSYS*|CYGWIN*)
    error "Windows detected; run scripts/install.ps1 in PowerShell instead"
    ;;
  *)
    error "unsupported operating system: $(uname -s)"
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) error "unsupported architecture: $(uname -m)" ;;
esac

if command -v curl >/dev/null 2>&1; then
  download() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  download() { wget -qO "$2" "$1"; }
else
  error "curl or wget is required"
fi

if [ "$version" = "latest" ]; then
  latest_url="https://api.github.com/repos/${REPO}/releases/latest"
  if command -v curl >/dev/null 2>&1; then
    if [ -n "${GITHUB_TOKEN:-}" ]; then
      response="$(curl -fsSL -H "Authorization: Bearer ${GITHUB_TOKEN}" "$latest_url")"
    else
      response="$(curl -fsSL "$latest_url")"
    fi
  else
    if [ -n "${GITHUB_TOKEN:-}" ]; then
      response="$(wget -qO- --header="Authorization: Bearer ${GITHUB_TOKEN}" "$latest_url")"
    else
      response="$(wget -qO- "$latest_url")"
    fi
  fi
  tag="$(printf '%s' "$response" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  [ -n "$tag" ] || error "could not resolve the latest release"
else
  tag="$version"
  case "$tag" in
    v*) ;;
    *) tag="v${tag}" ;;
  esac
fi
version="${tag#v}"

archive="${BINARY}_${version}_${os}_${arch}.tar.gz"
checksums="${BINARY}_${version}_checksums.txt"
base="https://github.com/${REPO}/releases/download/${tag}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

download "${base}/${archive}" "${tmp}/${archive}"
download "${base}/${checksums}" "${tmp}/${checksums}"

expected="$(awk -v file="$archive" '$2 == file { print $1; exit }' "${tmp}/${checksums}")"
[ -n "$expected" ] || error "checksum not found for ${archive}"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${tmp}/${archive}" | awk '{ print $1 }')"
else
  actual="$(shasum -a 256 "${tmp}/${archive}" | awk '{ print $1 }')"
fi
[ "$expected" = "$actual" ] || error "checksum verification failed for ${archive}"

mkdir -p "${tmp}/extract"
tar -xzf "${tmp}/${archive}" -C "${tmp}/extract"
[ -f "${tmp}/extract/${BINARY}" ] || error "archive does not contain ${BINARY}"

mkdir -p "$bin_dir"
[ -w "$bin_dir" ] || error "install directory is not writable: ${bin_dir} (use --bin-dir)"
cp "${tmp}/extract/${BINARY}" "${bin_dir}/${BINARY}"
chmod 0755 "${bin_dir}/${BINARY}"

profile_for_shell() {
  shell_name="$(basename "${SHELL:-sh}")"
  case "$shell_name" in
    zsh)
      printf '%s\n' "${HOME}/.zshrc"
      ;;
    fish)
      printf '%s\n' "${HOME}/.config/fish/config.fish"
      ;;
    bash)
      if [ -f "${HOME}/.bash_profile" ]; then
        printf '%s\n' "${HOME}/.bash_profile"
      elif [ -f "${HOME}/.bashrc" ]; then
        printf '%s\n' "${HOME}/.bashrc"
      else
        printf '%s\n' "${HOME}/.profile"
      fi
      ;;
    *)
      printf '%s\n' "${HOME}/.profile"
      ;;
  esac
}

case ":${PATH:-}:" in
  *":${bin_dir}:"*)
    echo "${bin_dir} is already on PATH"
    ;;
  *)
    if [ "$modify_path" -eq 1 ]; then
      profile="$(profile_for_shell)"
      mkdir -p "$(dirname "$profile")"
      marker="# Added by the ${BINARY} installer"
      if ! grep -qF "$marker" "$profile" 2>/dev/null; then
        case "$profile" in
          */config.fish)
            printf '\n%s\nfish_add_path "%s"\n' "$marker" "$bin_dir" >>"$profile"
            ;;
          *)
            printf '\n%s\nexport PATH="%s:$PATH"\n' "$marker" "$bin_dir" >>"$profile"
            ;;
        esac
      fi
      echo "Added ${bin_dir} to PATH in ${profile}"
      echo "Restart your shell or run: source ${profile}"
    else
      echo "Add ${bin_dir} to PATH manually:"
      case "${SHELL:-}" in
        */fish) echo "  fish_add_path \"${bin_dir}\"" ;;
        *) echo "  export PATH=\"${bin_dir}:\$PATH\"" ;;
      esac
    fi
    ;;
esac

echo "Installed ${BINARY} ${version} to ${bin_dir}/${BINARY}"
"${bin_dir}/${BINARY}" --version
