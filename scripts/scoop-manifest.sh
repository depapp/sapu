#!/bin/sh
# Writes bucket/sapu.json for Scoop from a release version and its SHA256SUMS.
# Usage: scripts/scoop-manifest.sh 0.2.0 path/to/SHA256SUMS > bucket/sapu.json
set -e
version=$1; sums=$2
[ -n "$version" ] && [ -f "$sums" ] || { echo "usage: $0 <version> <SHA256SUMS>" >&2; exit 1; }
hash() { awk -v f="$1" '$2 == f { print $1 }' "$sums"; }
base="https://github.com/depapp/sapu/releases/download"
cat <<JSON
{
    "version": "$version",
    "description": "Sweep your Android clean: deep clean, debloat, analyze and optimize your phone over adb, no root",
    "homepage": "https://github.com/depapp/sapu",
    "license": "GPL-3.0-only",
    "notes": "Needs adb on PATH: scoop install adb",
    "architecture": {
        "64bit": {
            "url": "$base/v$version/sapu-windows-amd64.exe#/sapu.exe",
            "hash": "$(hash sapu-windows-amd64.exe)"
        },
        "arm64": {
            "url": "$base/v$version/sapu-windows-arm64.exe#/sapu.exe",
            "hash": "$(hash sapu-windows-arm64.exe)"
        }
    },
    "bin": "sapu.exe",
    "checkver": "github",
    "autoupdate": {
        "architecture": {
            "64bit": { "url": "$base/v\$version/sapu-windows-amd64.exe#/sapu.exe" },
            "arm64": { "url": "$base/v\$version/sapu-windows-arm64.exe#/sapu.exe" }
        },
        "hash": { "url": "\$baseurl/SHA256SUMS" }
    }
}
JSON
