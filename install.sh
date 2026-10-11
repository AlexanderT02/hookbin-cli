#!/bin/sh
set -eu

repo="AlexanderT02/hookbin-cli"
install_dir="${HOOKBIN_INSTALL_DIR:-${HOME}/.local/bin}"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/hookbin-install.XXXXXX")"
step_pid=""
step_number=0
interactive=false
color=false
[ -t 1 ] && interactive=true
if [ "$interactive" = true ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-}" != dumb ]; then
  color=true
fi

cleanup() {
  if [ -n "$step_pid" ]; then
    kill "$step_pid" 2>/dev/null || true
    wait "$step_pid" 2>/dev/null || true
  fi
  rm -rf "$tmp_dir"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

if [ "$color" = true ]; then printf '\033[38;2;127;209;160m'; fi
printf '\n  ┌──────────────────────────────────┐\n'
printf '  │  hookbin  /  CLI installer       │\n'
printf '  └──────────────────────────────────┘\n'
if [ "$color" = true ]; then printf '\033[0m'; fi
printf '\n'

done_step() {
  if [ "$color" = true ]; then printf '  \033[38;2;127;209;160m✓\033[0m  %s\n' "$1"
  else printf '  ✓  %s\n' "$1"; fi
}

run_step() {
  label="$1"
  shift
  if [ "$interactive" != true ]; then
    printf '  →  %s...\n' "$label"
    "$@"
    done_step "$label"
    return
  fi

  step_number=$((step_number + 1))
  result_file="$tmp_dir/step-${step_number}.result"
  error_file="$tmp_dir/step-${step_number}.error"
  (
    outcome=failed
    trap 'printf "%s" "$outcome" >"$result_file"' EXIT
    if "$@" 2>"$error_file"; then outcome=ok; fi
  ) &
  step_pid=$!
  frame=0
  while [ ! -f "$result_file" ]; do
    case $((frame % 4)) in
      0) glyph='⠋' ;;
      1) glyph='⠙' ;;
      2) glyph='⠹' ;;
      3) glyph='⠸' ;;
    esac
    if [ "$color" = true ]; then printf '\r\033[2K  \033[38;2;127;209;160m%s\033[0m  %s...' "$glyph" "$label"
    else printf '\r  %s  %s...' "$glyph" "$label"; fi
    frame=$((frame + 1))
    sleep 0.12
  done
  wait "$step_pid"
  step_pid=""
  printf '\r\033[2K'
  if [ "$(cat "$result_file")" != ok ]; then
    printf '  ✗  %s failed.\n' "$label" >&2
    cat "$error_file" >&2
    return 1
  fi
  done_step "$label"
}

download() {
  url="$1"
  destination="$2"

  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --show-error "$url" --output "$destination"
  elif command -v wget >/dev/null 2>&1; then
    wget --quiet "$url" --output-document="$destination"
  else
    echo "hookbin installer requires curl or wget." >&2
    return 1
  fi
}

system="$(uname -s)"
binary="hookbin"
archive_extension="tar.gz"
case "$system" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  MINGW* | MSYS* | CYGWIN*)
    os="windows"
    binary="hookbin.exe"
    archive_extension="zip"
    ;;
  *)
    echo "Unsupported operating system: ${system}." >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *)
    echo "Unsupported CPU architecture: $(uname -m). Supported architectures are amd64 and arm64." >&2
    exit 1
    ;;
esac

run_step "Finding latest release" download "https://api.github.com/repos/${repo}/releases/latest" "$tmp_dir/release.json"
tag="$(sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_dir/release.json" | head -n 1)"
if [ -z "$tag" ]; then
  echo "Could not find a published hookbin release on GitHub." >&2
  exit 1
fi

version="${tag#v}"
archive="hookbin_${version}_${os}_${arch}.${archive_extension}"
release_url="https://github.com/${repo}/releases/download/${tag}"
run_step "Downloading hookbin ${tag}" download "${release_url}/${archive}" "$tmp_dir/$archive"
run_step "Downloading checksums" download "${release_url}/SHA256SUMS" "$tmp_dir/SHA256SUMS"

expected="$(awk -v file="$archive" '{ name = $2; sub(/^\.\//, "", name); sub(/^\*/, "", name); if (name == file) print $1 }' "$tmp_dir/SHA256SUMS")"
if [ -z "$expected" ]; then
  echo "No checksum found for ${archive}." >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp_dir/$archive" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$tmp_dir/$archive" | awk '{ print $1 }')"
else
  echo "hookbin installer requires sha256sum or shasum to verify the download." >&2
  exit 1
fi

if [ "$actual" != "$expected" ]; then
  echo "Checksum verification failed for ${archive}." >&2
  exit 1
fi
done_step "SHA-256 checksum verified"

if [ "$archive_extension" = "zip" ]; then
  if ! command -v unzip >/dev/null 2>&1; then
    echo "Git Bash needs unzip to install the Windows release archive." >&2
    exit 1
  fi
  run_step "Unpacking CLI" unzip -q "$tmp_dir/$archive" -d "$tmp_dir/extracted"
  extracted_dir="$tmp_dir/extracted"
else
  run_step "Unpacking CLI" tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"
  extracted_dir="$tmp_dir"
fi

mkdir -p "$install_dir"
cp "$extracted_dir/$binary" "$install_dir/$binary"
chmod 755 "$install_dir/$binary"

done_step "Installed hookbin ${tag}"
printf '\n  %s\n\n' "${install_dir}/${binary}"
if [ "$os" = "windows" ]; then
  printf '  Run it from Git Bash by path, as the inspector command does: ~/.local/bin/hookbin.exe listen ...\n'
  exit 0
fi
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) printf '  Add %s to PATH to run hookbin from any directory.\n' "$install_dir" ;;
esac
