#!/usr/bin/env bash
# Cross-compiles barecraft for every desktop platform into site/bin/
# so the GitHub Pages site can hand out platform-specific downloads.
set -euo pipefail
cd "$(dirname "$0")"

GO_BIN=${GO_BIN:-go}
OUT=bin
mkdir -p "$OUT"

build() {
  local goos=$1 goarch=$2 ext=$3
  echo "  -> barecraft-$goos-$goarch$ext"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    "$GO_BIN" build -trimpath -ldflags "-s -w" -o "$OUT/barecraft-$goos-$goarch$ext" .
}

echo "building barecraft:"
build linux   amd64 ""
build linux   arm64 ""
build darwin  amd64 ""
build darwin  arm64 ""
build windows amd64 ".exe"

echo
ls -la "$OUT"
