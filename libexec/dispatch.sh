#!/bin/sh
# Shared dispatcher: exec the native commercespine binary for this OS/arch.
#
# Invoked by the thin wrappers in bin/ as:
#   libexec/dispatch.sh <subcommand> [args...]
#
# It lives in libexec/ rather than bin/ because the plugin's bin/ directory is
# placed on PATH when the plugin is enabled — a helper there would show up as a
# runnable command.
#
# Strict POSIX sh: this must run under dash (/bin/sh on Debian/Ubuntu) and
# busybox sh (Alpine), not just bash. No 'exec -a', no [[ ]], no arrays.
set -eu

if [ "$#" -lt 1 ]; then
  echo "dispatch.sh: missing subcommand" >&2
  exit 1
fi
subcommand="$1"
shift

# Resolve this script's real directory, following symlinks.
src="$0"
while [ -h "$src" ]; do
  d="$(cd -P "$(dirname "$src")" && pwd)"
  src="$(readlink "$src")"
  case "$src" in /*) ;; *) src="$d/$src" ;; esac
done
LIBEXEC="$(cd -P "$(dirname "$src")" && pwd)"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux)  os=linux ;;
  MINGW*|MSYS*|CYGWIN*) os=windows ;;
  *) echo "commercespine: unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64)  arch=amd64 ;;
  *) echo "commercespine: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

exe="$LIBEXEC/commercespine-$os-$arch"
[ "$os" = windows ] && exe="$exe.exe"

if [ ! -x "$exe" ]; then
  echo "commercespine: no bundled binary for $os/$arch (looked for $exe)." >&2
  echo "commercespine: please report this to your Commerce Spine onboarding contact." >&2
  exit 1
fi

exec "$exe" "$subcommand" "$@"
