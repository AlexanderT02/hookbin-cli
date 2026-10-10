#!/bin/sh
set -eu

repo="AlexanderT02/hookbin"
install_dir="${HOOKBIN_INSTALL_DIR:-${HOME}/.local/bin}"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/hookbin-install.XXXXXX")"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT HUP INT TERM

download() {
  url="$1"
  destination="$2"

  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --show-error "$url" --output "$destination"
  elif command -v wget >/dev/null 2>&1; then
    wget --quiet "$url" --output-document="$destination"
  else
    echo "hookbin installer requires curl or wget." >&2
    exit 1
  fi
}

case "$(uname -s)" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *)
    echo "Unsupported operating system: $(uname -s). Use the Windows release archive from GitHub." >&2
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

download "https://api.github.com/repos/${repo}/releases/latest" "$tmp_dir/release.json"
tag="$(sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_dir/release.json" | head -n 1)"
if [ -z "$tag" ]; then
  echo "Could not find a published hookbin release on GitHub." >&2
  exit 1
fi

version="${tag#v}"
archive="hookbin_${version}_${os}_${arch}.tar.gz"
release_url="https://github.com/${repo}/releases/download/${tag}"
download "${release_url}/${archive}" "$tmp_dir/$archive"
download "${release_url}/SHA256SUMS" "$tmp_dir/SHA256SUMS"

expected="$(awk -v file="$archive" '$2 == file { print $1 }' "$tmp_dir/SHA256SUMS")"
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

tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"
mkdir -p "$install_dir"
cp "$tmp_dir/hookbin" "$install_dir/hookbin"
chmod 755 "$install_dir/hookbin"

echo "Installed hookbin ${tag} to ${install_dir}/hookbin"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) echo "Add ${install_dir} to your PATH to run 'hookbin' from any directory." ;;
esac
