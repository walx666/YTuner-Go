#!/usr/bin/env sh
# ---------------------------------------------------------------------------
# YTuner (Go port) build script - Linux/macOS/BSD and MSYS2 (mingw64) shell.
#
# Usage: ./build.sh [build|test|cross|dist|sql|clean]   (default: build)
#   build   host binary            -> bin/ytuner[.exe]
#   test    go vet + unit tests
#   cross   all release targets    -> bin/<os>-<arch>/ytuner[.exe]
#   dist    cross + sample config next to each binary
#   sql     re-sync embedded SQL scripts from ../../src/sql
#   clean   remove bin/
#
# Go is taken from PATH; in MSYS2 install it with: pacman -S mingw-w64-x86_64-go
# ---------------------------------------------------------------------------
set -eu
cd "$(dirname "$0")"

CMD="${1:-build}"

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    # MSYS2's Go is "trimmed" and needs GOROOT; Windows paths for GOPATH/GOCACHE.
    if [ -z "${GOROOT:-}" ] && [ -d /mingw64/lib/go ]; then export GOROOT=/mingw64/lib/go; fi
    if ! command -v go >/dev/null 2>&1 && [ -x /mingw64/bin/go ]; then PATH="/mingw64/bin:$PATH"; fi
    : "${GOPATH:=$(cygpath -m "${USERPROFILE:-$HOME}")/go}"
    : "${GOCACHE:=$(cygpath -m "${LOCALAPPDATA:-$HOME/.cache}")/go-build}"
    export GOPATH GOCACHE
    EXE=.exe ;;
  *) EXE= ;;
esac
export CGO_ENABLED=0

command -v go >/dev/null 2>&1 || { echo "ERROR: go not found (MSYS2: pacman -S mingw-w64-x86_64-go)"; exit 1; }

TARGETS="linux/amd64 linux/arm64 linux/arm linux/386 windows/amd64 windows/386 windows/arm64 darwin/amd64 darwin/arm64 freebsd/amd64"
LDFLAGS="-s -w"

case "$CMD" in
  build)
    go build -trimpath -ldflags "$LDFLAGS" -o "bin/ytuner$EXE" ./cmd/ytuner
    echo "OK: bin/ytuner$EXE" ;;
  test)
    go vet ./...
    go test -count=1 ./... ;;
  cross|dist)
    for t in $TARGETS; do
      os=${t%/*}; arch=${t#*/}; ext=""; [ "$os" = windows ] && ext=".exe"
      echo "building $os/$arch"
      GOOS=$os GOARCH=$arch GOARM=7 go build -trimpath -ldflags "$LDFLAGS" -o "bin/$os-$arch/ytuner$ext" ./cmd/ytuner
      if [ "$CMD" = dist ]; then
        mkdir -p "bin/$os-$arch/config"
        cp ../../cfg/ytuner.ini "bin/$os-$arch/"
        cp ../../cfg/avr.ini ../../cfg/stations.ini ../../cfg/stations.yaml "bin/$os-$arch/config/"
      fi
    done ;;
  sql)
    cp ../../src/sql/*.sql internal/ytuner/sql/ ;;
  clean)
    rm -rf bin ;;
  -h|--help) sed -n '2,13p' "$0" ;;
  *) echo "Unknown command: $CMD" >&2; exit 2 ;;
esac
