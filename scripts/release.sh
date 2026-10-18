#!/usr/bin/env bash
# Сборка архивов релиза для всех платформ: scripts/release.sh v0.1.0 → dist/
set -euo pipefail

VERSION="${1:?укажите версию, например v0.1.0}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
TARGETS="linux/amd64 linux/arm64 linux/arm windows/amd64 windows/arm64 darwin/amd64 darwin/arm64"

rm -rf "$DIST"
mkdir -p "$DIST"
cd "$ROOT"

for target in $TARGETS; do
	os="${target%/*}"
	arch="${target#*/}"
	name="slamsim_${VERSION}_${os}_${arch}"
	if [ "$arch" = "arm" ]; then
		name="${name}v7"
	fi
	stage="$DIST/$name"
	ext=""
	if [ "$os" = "windows" ]; then
		ext=".exe"
	fi
	mkdir -p "$stage/data"

	export CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOARM=7
	go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$stage/slamsim$ext" ./cmd/slamsim
	go build -trimpath -ldflags "-s -w" -o "$stage/slamsim-generate$ext" ./cmd/generate

	cp README.md "$stage/"
	cp -r python "$stage/python"
	find "$stage/python" -name __pycache__ -prune -exec rm -rf {} +
	: > "$stage/data/pats.bin"

	(
		cd "$DIST"
		if [ "$os" = "windows" ]; then
			zip -qr "$name.zip" "$name"
		else
			tar -czf "$name.tar.gz" "$name"
		fi
	)
	rm -rf "$stage"
	echo "собран $name"
done

# Отдельно — Python-клиент, если нужен только он (например, на роботе)
(cd "$ROOT/python" && zip -qr "$DIST/rrosim_${VERSION}_python.zip" rrosim pyproject.toml examples -x '*__pycache__*')

(cd "$DIST" && sha256sum -- * > SHA256SUMS.txt)
echo "готово: $DIST"
