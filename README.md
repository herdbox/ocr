# herdbox-ocr

[![build](https://github.com/herdbox/ocr/actions/workflows/build.yml/badge.svg)](https://github.com/herdbox/ocr/actions/workflows/build.yml)
[![release](https://img.shields.io/github/v/release/herdbox/ocr)](https://github.com/herdbox/ocr/releases)
[![license](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)

Offline OCR for Android phone screenshots, packaged as one process you talk to over stdin/stdout. [Herdbox](https://github.com/herdbox) nodes on Linux and Windows use it to read text on screen. macOS nodes don't need it; they use Apple Vision.

It finds text lines with the PP-OCRv5 mobile detection model on ONNX Runtime, then reads each line with Tesseract 5 and the `vie` model from tessdata_best. That covers Vietnamese with tone marks, and English. Chinese and other scripts are not supported.

On our test set of 173 Vietnamese and English UI lines (12 synthetic screens and a real phone screenshot), 169 (97.7 %) come back exactly right. A 1080×2400 screenshot takes about 240 ms once warm, and the process peaks at about 310 MB of memory (linux/arm64, Docker).

## Install

Each release ships one zip per platform plus a `SHA256SUMS` file:

| Platform | Asset | Requires |
| --- | --- | --- |
| Linux x86-64 | `herdbox-ocr-<N>-linux-amd64.zip` | glibc 2.35+ (Ubuntu 22.04, Debian 12 or newer) |
| Linux arm64 | `herdbox-ocr-<N>-linux-arm64.zip` | glibc 2.35+ |
| Windows x64 | `herdbox-ocr-<N>-windows-amd64.zip` | Windows 10 1809+ or 11 |
| Windows arm64 | `herdbox-ocr-<N>-windows-arm64.zip` | Windows 10 1809+ or 11 |

Download, check, unzip:

```sh
N=1
base=https://github.com/herdbox/ocr/releases/download/v$N
curl -fLO $base/herdbox-ocr-$N-linux-amd64.zip -fLO $base/SHA256SUMS
sha256sum -c --ignore-missing SHA256SUMS
unzip herdbox-ocr-$N-linux-amd64.zip -d herdbox-ocr
```

Keep the files together. The program looks for its model, tessdata and the ONNX Runtime library next to its own executable (symlinks are followed):

```
herdbox-ocr[.exe]           Tesseract and Leptonica are linked in statically
libonnxruntime.so.1.31.1    onnxruntime.dll on Windows, plus the VC++ runtime DLLs
det.onnx
tessdata/vie.traineddata
LICENSES/
VERSION
```

You don't need to install a VC++ redistributable on Windows.

## Usage

Start the program with no arguments. It loads everything, then prints one handshake line on stdout:

```
herdbox-ocr 1 7
```

That's the protocol version (`1`) and the package version (`7`). If loading fails, it prints `herdbox-ocr-error <message>` and exits with code 1.

After the handshake, all traffic in both directions is in frames. Send PNG screenshots and get text lines back. Here is a minimal Python client:

```python
import json, struct, subprocess

def send(p, header, body=b""):
    h = json.dumps(header).encode()
    p.stdin.write(struct.pack(">I", len(h)) + h + struct.pack(">I", len(body)) + body)
    p.stdin.flush()

def recv(p):
    def part():
        n = struct.unpack(">I", p.stdout.read(4))[0]
        return p.stdout.read(n)
    header = json.loads(part())
    part()  # answers have an empty body
    return header

p = subprocess.Popen(["./herdbox-ocr"], stdin=subprocess.PIPE, stdout=subprocess.PIPE)
print(p.stdout.readline().decode().strip())

png = open("screen.png", "rb").read()
send(p, {"id": 1, "op": "ocr", "region": [0, 0, 1080, 2400]}, png)
for item in recv(p)["items"]:
    print(item["text"], item["confidence"], item["bounds"])

p.stdin.close()
p.wait()
```

On a phone settings screen the output looks like this:

```
herdbox-ocr 1 0
Cài đặt 0.96 [57, 300, 473, 445]
Tìm kiếm 0.96 [173, 540, 352, 595]
Wi-Fi 0.95 [224, 1345, 372, 1420]
Mạng di động 0.95 [224, 1512, 553, 1600]
› 0.43 [957, 1530, 989, 1575]
...
```

Icons and chevrons get detected as text too, and come back as short junk like `›` or `@`, usually with low confidence. Clean UI labels in that screenshot scored 0.92 to 0.96, but a real line can score low too (a truncated `FindMy..` got 0.48). Matching the strings you expect works better than a fixed confidence cutoff.

## Protocol

This is version 1. A client should refuse a handshake with a protocol version it doesn't know.

### Frames

Every message, in both directions, is:

```
uint32 big-endian  header length
bytes              header, a JSON object
uint32 big-endian  body length
bytes              body (may be empty)
```

Headers can be up to 64 KiB and bodies up to 64 MiB. If a frame is malformed or too large, or its header isn't valid JSON, the program treats the stream as broken. It finishes the requests in flight and exits with code 2.

### Requests

`ocr` reads text inside a region of a PNG, sent as the body:

```json
{"id": 17, "op": "ocr", "region": [x0, y0, x1, y1]}
```

`region` is in image pixels, `x1`/`y1` exclusive, and gets clipped to the image. For the whole screen, pass the image size or anything larger. A region that doesn't overlap the image gives an empty list.

`ping` checks that the process is responsive. It has no body:

```json
{"id": 18, "op": "ping"}
```

`id` is an integer you choose. It comes back unchanged in the answer.

### Answers

Answers have an empty body and can come back in any order. Match them by `id`.

```json
{"id": 17, "items": [{"text": "Cài đặt", "confidence": 0.96, "bounds": [57, 300, 473, 445]}]}
{"id": 18, "ok": true}
{"id": 17, "error": "bad_image", "message": "cannot decode PNG"}
```

Items are text lines, top to bottom and then left to right. `bounds` is `[x0, y0, x1, y1]` in pixels of the full image, not of the region. `confidence` is Tesseract's mean word confidence scaled to 0–1.

`error` is either `bad_image` (the body isn't a PNG) or `internal` (detection failed, or the op is unknown). Neither ends the process.

### Concurrency and shutdown

You can send new requests without waiting for answers. Up to `min(CPUs, 4)` lines are read in parallel, across all requests in flight. When you close stdin, the program answers everything it has received and exits with code 0.

## How it works

1. The region is resized so its long side is 960 px, and the PP-OCRv5 mobile det model turns it into a text probability map.
2. A DB-style post-processor thresholds the map and collects connected regions. It drops small or low-scoring boxes, merges the boxes on one line and grows them back out, stopping halfway to the lines above and below. Boxes are axis-aligned, so rotated text is not handled.
3. Each line crop (plus 4 px of padding) goes to a Tesseract worker in single-line mode.

Model files and the ONNX Runtime build are pinned by URL and sha256 in `build/`.

## Building from source

You need cgo with a static Tesseract and Leptonica, built by [vcpkg](https://vcpkg.io). The scripts in `build/` do all of this.

Linux, the easy way, in Docker:

```sh
docker build -f Dockerfile.dev -t herdbox-ocr-dev .   # builds the package into /out
docker run --rm herdbox-ocr-dev                        # runs every test against it
```

Linux, natively (Go 1.27, a C/C++ toolchain, pkg-config, curl, zip; `VCPKG_ROOT` set):

```sh
build/build.sh   # → out/herdbox-ocr-0-linux-<arch>.zip and out/pkg-linux-<arch>/
build/test.sh    # unit and end-to-end tests against out/pkg-linux-<arch>/
```

Windows: run `build/build.ps1` in PowerShell 7 with [llvm-mingw](https://github.com/mstorsjo/llvm-mingw) on `PATH`, Git for Windows, and Visual Studio's VC++ redist files. cgo needs the MinGW ABI, so MSVC alone won't work.

`VERSION=<N>` sets the package version (default `0`), and `GOARCH` picks the target architecture.

## Tests

```sh
build/test.sh                          # everything
build/test.sh -run DetPost             # post-processing unit tests only
build/test.sh -run VietnameseSet -v    # accuracy over testdata/synthetic
```

The end-to-end tests run with `-tags e2e` and need a built package in `HERDBOX_OCR_PKG_DIR`. `TestOCR_VietnameseSet` fails if fewer than 95 % of the lines in `testdata/*/gt.json` come back exactly.

The synthetic screens in `testdata/synthetic/` come from `gen.py` (fixed seed). To change them, edit the script and regenerate rather than touching the PNGs.

## Releases

Push a `v<N>` tag, where N is an integer. The [build workflow](.github/workflows/build.yml) builds and tests all four platforms, checks the Linux packages on a stock `debian:12`, and attaches the zips and `SHA256SUMS` to the release. Tags with `-rc` in them (`v2-rc1`) become prereleases of package N.

Herdbox core pins N and the four checksums, so a published release should never be rebuilt in place.

## License

herdbox-ocr is licensed under the [AGPL-3.0](LICENSE).

The packages bundle third-party components under their own licenses. Each zip includes the full texts in `LICENSES/`.

| Component | License |
| --- | --- |
| ONNX Runtime | MIT |
| Tesseract | Apache-2.0 |
| Leptonica | BSD-2-Clause |
| PP-OCRv5 mobile det model | Apache-2.0 |
| tessdata_best `vie` | Apache-2.0 |
