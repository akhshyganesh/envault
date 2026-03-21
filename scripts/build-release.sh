#!/usr/bin/env bash
# build-release.sh — cross-compile envault for all platforms
set -e

VERSION=${1:-"dev"}
DIST="dist"
mkdir -p "$DIST"

targets=(
  "linux   amd64"
  "linux   arm64"
  "darwin  amd64"
  "darwin  arm64"
)

for target in "${targets[@]}"; do
  GOOS=$(echo "$target" | awk '{print $1}')
  GOARCH=$(echo "$target" | awk '{print $2}')
  OUTPUT="$DIST/envault-${GOOS}-${GOARCH}"

  echo "Building $OUTPUT..."
  env CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -ldflags="-s -w -X main.version=${VERSION}" \
    -o "$OUTPUT" .
done

echo ""
echo "Creating checksums..."
cd "$DIST"
shasum -a 256 envault-* > checksums.txt
cat checksums.txt

echo ""
echo "Done! Binaries are in $DIST/"
ls -lh envault-*
