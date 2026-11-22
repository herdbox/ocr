#!/usr/bin/env bash
# Builds the herdbox-ocr package for linux/$GOARCH (default: host) into $OUT
# (default ./out): vcpkg static Tesseract + Leptonica, ONNX Runtime from
# Microsoft's NuGet package, models from fetch-models.sh.
#
# Needs VCPKG_ROOT, Go, a C/C++ toolchain, pkg-config, curl, unzip, zip.
# VERSION is the package version N (default 0 = dev build).
set -euo pipefail

source "$(dirname "$0")/cgo-env.sh"
version=${VERSION:-0}
license=${LICENSE_FILE:-$src/LICENSE}

ORT_VERSION=1.31.1
ORT_URL=https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/$ORT_VERSION/microsoft.ml.onnxruntime.$ORT_VERSION.nupkg
ORT_SHA256=6871ccbc97f3d5bdb4095a1dcec33aa78d56f6ed8de5980ef5fa1d9f2e7c679a
: "${VCPKG_ROOT:?set VCPKG_ROOT}"

mkdir -p "$out"
"$VCPKG_ROOT/vcpkg" install --x-manifest-root="$here" --x-install-root="$installed" --triplet "$triplet"
cgo_flags

"$here/fetch-models.sh" "$out/models"

nupkg=$out/ort-$ORT_VERSION.nupkg
if [ ! -f "$nupkg" ] || [ "$(sha256sum "$nupkg" | cut -d' ' -f1)" != "$ORT_SHA256" ]; then
	curl -fsSL --retry 3 -o "$nupkg" "$ORT_URL"
	echo "$ORT_SHA256  $nupkg" | sha256sum -c -
fi

pkg=$out/pkg-linux-$arch
rm -rf "$pkg"
mkdir -p "$pkg/tessdata" "$pkg/LICENSES"
unzip -p "$nupkg" "runtimes/$rid/native/libonnxruntime.so" >"$pkg/libonnxruntime.so.$ORT_VERSION"
(cd "$src" && GOARCH=$arch go build -trimpath -ldflags="-s -w" -o "$pkg/herdbox-ocr" .)
chmod 0755 "$pkg/herdbox-ocr"
cp "$out/models/det.onnx" "$pkg/"
cp "$out/models/tessdata/vie.traineddata" "$pkg/tessdata/"
unzip -p "$nupkg" LICENSE >"$pkg/LICENSES/onnxruntime.txt"
unzip -p "$nupkg" ThirdPartyNotices.txt >"$pkg/LICENSES/onnxruntime-third-party.txt"
cp "$lib/share/tesseract/copyright" "$pkg/LICENSES/tesseract.txt"
cp "$lib/share/leptonica/copyright" "$pkg/LICENSES/leptonica.txt"
for p in libpng libjpeg-turbo tiff zlib; do
	if [ -f "$lib/share/$p/copyright" ]; then cp "$lib/share/$p/copyright" "$pkg/LICENSES/$p.txt"; fi
done
cp "$src/LICENSE-models.txt" "$pkg/LICENSES/models.txt"
cp "$license" "$pkg/LICENSES/herdbox-ocr.txt"
echo "$version" >"$pkg/VERSION"

zipfile=$out/herdbox-ocr-$version-linux-$arch.zip
rm -f "$zipfile"
(cd "$pkg" && zip -qr -X "$zipfile" .)
echo "$zipfile"
