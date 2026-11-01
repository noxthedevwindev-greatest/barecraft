#!/bin/sh
# barecraft one-liner helper — https://noxthedevwindev-greatest.github.io/barecraft/
# usage: curl -fsSL .../i.sh | bash -s -- <base64url config>
set -e

if [ -z "$1" ]; then
  echo "barecraft: no config given" >&2
  echo "make one on the barecraft site and paste the full line" >&2
  exit 1
fi

B="${BARECRAFT_BASE:-https://noxthedevwindev-greatest.github.io/barecraft/bin}"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  *) os=linux ;;
esac

a="$(uname -m)"
case "$a" in
  aarch64|arm64) a=arm64 ;;
  *) a=amd64 ;;
esac

tmp="${TMPDIR:-/tmp}/barecraft-$$"

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$B/barecraft-$os-$a" -o "$tmp"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$tmp" "$B/barecraft-$os-$a"
else
  echo "barecraft: need curl or wget" >&2
  exit 1
fi

chmod +x "$tmp"
set +e
"$tmp" "$@"
rc=$?
rm -f "$tmp"
exit "$rc"
