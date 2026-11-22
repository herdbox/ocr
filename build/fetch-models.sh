#!/usr/bin/env bash
# Downloads the pinned det model and Tesseract vie data into $1 (default
# ./models) and checks their sha256.
set -euo pipefail

dest=${1:-models}
DET_URL=https://huggingface.co/PaddlePaddle/PP-OCRv5_mobile_det_onnx/resolve/e6f4fa85f00e168c862bc462aebca69eef9b3d3d/inference.onnx
DET_SHA256=a431985659dc921974177a95adcfbb90fd9e51989a5e04d70d0b75f597b6e61d
VIE_URL=https://raw.githubusercontent.com/tesseract-ocr/tessdata_best/e12c65a915945e4c28e237a9b52bc4a8f39a0cec/vie.traineddata
VIE_SHA256=b6b49293d95d0b6dbd8780174627e82c75be957b6f4ed9862155540d6b00bb45

sha256() {
	if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

fetch() { # url sha file
	if [ -f "$3" ] && [ "$(sha256 "$3")" = "$2" ]; then return; fi
	curl -fsSL --retry 3 -o "$3.part" "$1"
	got=$(sha256 "$3.part")
	if [ "$got" != "$2" ]; then
		rm -f "$3.part"
		echo "sha256 mismatch for $1: got $got, want $2" >&2
		exit 1
	fi
	mv "$3.part" "$3"
}

mkdir -p "$dest/tessdata"
fetch "$DET_URL" "$DET_SHA256" "$dest/det.onnx"
fetch "$VIE_URL" "$VIE_SHA256" "$dest/tessdata/vie.traineddata"
