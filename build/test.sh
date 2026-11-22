#!/usr/bin/env bash
# Runs herdbox-ocr's tests, end-to-end ones included, against the package
# build.sh left in $OUT. HERDBOX_OCR_FIXTURES: folder with settings.png
# (default: testdata/).
set -euo pipefail
source "$(dirname "$0")/cgo-env.sh"
cgo_flags
export HERDBOX_OCR_PKG_DIR=${HERDBOX_OCR_PKG_DIR:-$out/pkg-linux-$arch}
cd "$src"
go test -count=1 -tags e2e -v "$@" ./...
