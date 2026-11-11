#!/bin/sh
# Install the latest Sapu release: curl -fsSL https://raw.githubusercontent.com/depapp/sapu/main/install.sh | sh
# Verifies SHA256SUMS. Set SAPU_INSTALL_DIR to choose the directory (default: /usr/local/bin, else ~/.local/bin).
set -eu

repo="depapp/sapu"
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$os" in
  darwin|linux) ;;
  mingw*|msys*|cygwin*) os=windows ;;
  *) echo "sapu: unsupported OS: $os (download a binary from https://github.com/$repo/releases)"; exit 1 ;;
esac
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "sapu: unsupported CPU: $arch"; exit 1 ;;
esac
asset="sapu-$os-$arch"
[ "$os" = windows ] && asset="$asset.exe"

# Resolve the latest tag from the releases/latest redirect (no API rate limit); fall back to the API.
tag=$(curl -fsSI -o /dev/null -w '%{redirect_url}' "https://github.com/$repo/releases/latest" | sed -n 's#.*/releases/tag/\([^/?]*\).*#\1#p')
[ -n "$tag" ] || tag=$(curl -fsSL -H 'Accept: application/vnd.github+json' "https://api.github.com/repos/$repo/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
[ -n "$tag" ] || { echo "sapu: no release found at https://github.com/$repo/releases"; exit 1; }
base="https://github.com/$repo/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "Downloading sapu $tag ($asset)..."
curl -fsSL -o "$tmp/$asset" "$base/$asset"
curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS"
want=$(grep " \*\{0,1\}$asset\$" "$tmp/SHA256SUMS" | awk '{print tolower($1)}')
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmp/$asset" | awk '{print $1}')
else got=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}'); fi
[ "$want" = "$got" ] || { echo "sapu: checksum mismatch for $asset, aborting"; exit 1; }
chmod 755 "$tmp/$asset"

dir="${SAPU_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then dir=/usr/local/bin; else dir="$HOME/.local/bin"; fi
fi
mkdir -p "$dir"
mv "$tmp/$asset" "$dir/sapu"
echo "Installed $dir/sapu ($tag)"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "Add $dir to your PATH, e.g.: export PATH=\"$dir:\$PATH\"" ;;
esac
command -v adb >/dev/null 2>&1 || echo "adb not found: brew install android-platform-tools | sudo apt install adb | scoop install adb"
echo "Run: sapu"
