#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$ROOT/dist"

if [ "$(go env GOOS)" = "windows" ]; then
  go -C "$ROOT/app" build -ldflags="-H=windowsgui" -o "$ROOT/dist/clashvless.exe" .
  go -C "$ROOT/app" build -o "$ROOT/dist/clashvless-cli.exe" .
else
  go -C "$ROOT/app" build -o "$ROOT/dist/clashvless" .
fi
