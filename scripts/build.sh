#!/bin/sh
# Cross-compile the commercespine binary for every supported platform.
#
# CGO_ENABLED=0 is the important part: it produces a fully static binary with no
# libc dependency, so one Linux build runs on glibc (Debian/Ubuntu/RHEL) *and*
# musl (Alpine) without "GLIBC_2.xx not found" errors.
#
# Usage: scripts/build.sh [version]

set -eu

VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/libexec"

# GOOS/GOARCH pairs. Add or remove lines to change the shipped matrix.
TARGETS="
darwin/arm64
darwin/amd64
linux/amd64
linux/arm64
windows/amd64
windows/arm64
"

mkdir -p "$OUT"
rm -f "$OUT"/commercespine-*

echo "Building commercespine $VERSION"
for target in $TARGETS; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"
  name="commercespine-$GOOS-$GOARCH"
  [ "$GOOS" = "windows" ] && name="$name.exe"

  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
    -o "$OUT/$name" "$ROOT/cmd/commercespine"

  chmod 0755 "$OUT/$name"
  size=$(du -h "$OUT/$name" | cut -f1 | tr -d ' ')
  printf '  %-32s %s\n' "$name" "$size"
done

echo "Done. Total: $(du -sh "$OUT" | cut -f1 | tr -d ' ')"
