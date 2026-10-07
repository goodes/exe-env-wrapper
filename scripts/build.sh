#!/usr/bin/env bash
# Build every released binary into $OUT (default: dist/).
#
# Usage: scripts/build.sh [version]
#
# Windows gets two variants from the same source. The console build is for
# interactive use and for agents that already own a console. The -w build is
# linked into the GUI subsystem so Windows never gives it a console of its own,
# and it spawns the target with CREATE_NO_WINDOW so no window flashes either.
set -euo pipefail

version="${1:-dev}"
out="${OUT:-dist}"
pkg=./cmd/exe-env-wrapper
stamp="-X github.com/goodes/exe-env-wrapper/internal/wrapper.Version=${version}"

rm -rf "$out"
mkdir -p "$out"

# build <goos> <goarch> <console|windowless> <output name>
build() {
  local goos="$1" goarch="$2" mode="$3" name="$4"
  local tags=() ldflags="-s -w $stamp"

  if [ "$mode" = windowless ]; then
    tags=(-tags windowsgui)
    ldflags="$ldflags -H windowsgui"
  fi

  echo "building $name ($goos/$goarch, $mode)"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath "${tags[@]}" -ldflags "$ldflags" -o "$out/$name" "$pkg"
}

build windows amd64 console    exe-env-wrapper-windows-amd64.exe
build windows amd64 windowless exe-env-wrapper-w-windows-amd64.exe
build windows arm64 console    exe-env-wrapper-windows-arm64.exe
build windows arm64 windowless exe-env-wrapper-w-windows-arm64.exe
build linux   amd64 console    exe-env-wrapper-linux-amd64
build linux   arm64 console    exe-env-wrapper-linux-arm64

( cd "$out" && sha256sum ./* > SHA256SUMS )
echo
echo "artifacts in $out:"
ls -l "$out"
