#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$ROOT/dist"

if [ "$(go env GOOS)" = "windows" ]; then
  if ! command -v gcc >/dev/null 2>&1 && ! command -v clang >/dev/null 2>&1; then
    for p in /c/msys64/mingw64/bin /c/msys64/ucrt64/bin /c/mingw64/bin C:/msys64/mingw64/bin C:/msys64/ucrt64/bin; do
      if [ -x "$p/gcc.exe" ] || [ -x "$p/gcc" ]; then
        export PATH="$p:$PATH"
        export CGO_ENABLED=1
        break
      fi
    done
  fi
  go -C "$ROOT/app" build -ldflags="-H=windowsgui" -o "$ROOT/dist/clashvless.exe" .
  go -C "$ROOT/app" build -o "$ROOT/dist/clashvless-cli.exe" .
else
  go -C "$ROOT/app" build -o "$ROOT/dist/clashvless" .
fi
