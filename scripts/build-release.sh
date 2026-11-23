#!/usr/bin/env bash
# Cross-compiles the relay for every released platform and writes the release assets with their checksums:
#   scripts/build-release.sh <version> <out dir>
# Each asset is protobase-relay-<GOOS>-<GOARCH>, with .exe on Windows, and reports <version> with -version.
# <out dir>/SHA256SUMS lists every asset in sha256sum's format, so `sha256sum -c SHA256SUMS` checks them.
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: scripts/build-release.sh <version> <out dir>" >&2
  exit 2
fi
version=$1
out=$2

platforms=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64)

root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)

assets=()
for platform in "${platforms[@]}"; do
  goos=${platform%/*}
  goarch=${platform#*/}
  asset="protobase-relay-$goos-$goarch"
  if [ "$goos" = windows ]; then
    asset="$asset.exe"
  fi
  (cd "$root" && CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch \
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$out/$asset" ./cmd/relay)
  assets+=("$asset")
done

(cd "$out" && sha256sum "${assets[@]}" > SHA256SUMS)
