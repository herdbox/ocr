# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`herdbox-ocr`: standalone OCR process for Herdbox nodes on Linux and Windows (macOS nodes use Apple Vision). Go + cgo, module `github.com/herdbox/ocr`. The client side (lookup, download, sha256 pin, process lifecycle) lives in core's `automation/ocr`.

## Spec

`/Users/reu98/Work/herdbox/docs/srs/core/modules/locate-vision-ocr.md`, FR-OCR-01…05. Read it before changing behavior. This repo is bound by FR-OCR-01 (model choice), FR-OCR-04 (platform floor, tag/asset names) and FR-OCR-05 (the client's 30 s per-request timeout). `SC-001a` (≥ 95 % exact lines, cited in `e2e_test.go`) is not in the docs. Ask before you change it or rely on it.

## Contracts: coordinate before changing

Each of these is pinned on the client side in core. Changing one breaks deployed nodes. Say so and wait, and ask the `core` session (via SendMessage) for the matching change.

- **Process protocol** (`README.md`, `frame.go`, `main.go`): handshake line, framing, request/answer JSON, error codes `bad_image` / `internal`, exit on stdin EOF. Any change bumps `protocol` in `main.go`. Core rejects versions it doesn't accept and treats the copy as broken (FR-OCR-03).- **Release names**: tag `v<N>` (integer), assets `herdbox-ocr-<N>-<goos>-<goarch>.zip` + `SHA256SUMS`. Core pins N and the four sha256 values in `automation/ocr/pin.go`. A rebuild changes the sums.
- **Package layout**: everything sits next to the executable (symlinks resolved). That means `det.onnx`, `tessdata/vie.traineddata`, the ORT library (`libonnxruntime.so.1.31.1` / `onnxruntime.dll`), `VERSION`, `LICENSES/`, and on Windows the VC++ runtime DLLs.

## Architecture

Per `ocr` request: decode PNG → clip `region` → detect line boxes → fan boxes out to reader workers → keep non-empty lines.

- `frame.go`: uint32 BE length + JSON header, then uint32 BE length + body. Limits are 64 KiB / 64 MiB. A bad frame or header makes `serve` exit with code 2.
- `main.go`: frames are read sequentially and each `ocr` request runs in its own goroutine. Answers are written under a mutex, in any order. On EOF it drains the requests in flight.
- `det.go`: PP-OCRv5 mobile det via `onnxruntime_go`. A single session is shared, since ORT `Run` is concurrency-safe. Input: long side 960, sides multiples of 32, BGR, PaddleOCR normalization.
- `dbpost.go`: DB post-processing, **axis-aligned only**: threshold → 8-connected components (union-find) → size/score filter → merge same-line boxes → unclip. Tuning constants are at the top.
- `tess.go`: Tesseract C API, `vie`, `PSM_SINGLE_LINE`. `TessBaseAPI` is not thread-safe, so each worker owns one `reader`. There are `min(NumCPU, 4)` workers. `OMP_THREAD_LIMIT=1` is set and ORT runs single-threaded, so parallelism comes only from workers and requests.

## Commands

cgo needs Tesseract headers and libs. `build/cgo-env.sh` (sourced by `build.sh` / `test.sh`) sets them from vcpkg. A plain `go build` fails.

```sh
# Mac quick loop (brew tesseract; no ORT/package, so unit tests only)
export CGO_CFLAGS="$(pkg-config --cflags tesseract)" CGO_LDFLAGS="$(pkg-config --libs tesseract)"
go vet . && go vet -tags e2e .
go test -count=1 -run DetPost -v .

# Full check: Linux in Docker, builds the package into /out and runs build/test.sh (e2e included)
docker build -f Dockerfile.dev -t herdbox-ocr-dev .
docker run --rm herdbox-ocr-dev

# Native Linux (VCPKG_ROOT, Go, C/C++ toolchain, pkg-config)
build/build.sh                               # → out/herdbox-ocr-<VERSION>-linux-<arch>.zip + out/pkg-linux-<arch>/
build/test.sh -run TestFROCR01_SettingsFixture   # args pass through to go test

# Windows: llvm-mingw on PATH (cgo needs the MinGW ABI), vcpkg *-mingw-static
build/build.ps1
```

Env vars: `VERSION` (package N, default 0), `GOARCH`, `OUT`, `VCPKG_INSTALLED`, `HERDBOX_OCR_PKG_DIR` (e2e package dir), `HERDBOX_OCR_FIXTURES` (default `testdata`), `LICENSE_FILE` (default `./LICENSE`).

CI (`.github/workflows/build.yml`) builds and tests all four platforms, checks the Linux package on `debian:12` (glibc floor), and publishes on a `v<N>` tag (`-rc` makes it a prerelease). `build.ps1` and the workflow haven't run yet.

## Verify: what "done" means here

Pick the check that matches the change and report the numbers:

| Change | Done when |
| --- | --- |
| `dbpost.go` / `det.go` / `tess.go` / preprocessing | Docker full check passes **and** you report `TestOCR_VietnameseSet` before vs. after. The baseline is 164/168 = 97.6 % and `SettingsFixture` 7/7. A drop is a regression, even if it stays ≥ 95 %. |
| Pure post-processing logic | Add or adjust a `TestFROCR01_DetPost*` case, then run the Mac quick loop. |
| `frame.go` / `main.go` | Docker full check passes, and the protocol is unchanged (or you've coordinated per *Contracts*). |
| Build scripts / Dockerfile / pins | Docker full check passes. For Windows-only or CI-only edits, say they're unverified locally. |

Performance figures are printed by `TestFROCR01_SettingsFixture`. On linux/arm64 Docker: cold ~490 ms, warm ~240 ms, peak RSS ~311 MB. Mention big shifts.

## Pins: change together

- ONNX Runtime 1.31.1 (URL + sha256): `build/build.sh`, `build/build.ps1`, `ortLibrary()` in `det.go`.
- Det model and `vie.traineddata` (URL + sha256): `build/fetch-models.sh`.
- vcpkg commit: `Dockerfile.dev`, `builtin-baseline` in `build/vcpkg.json`, `VCPKG_COMMIT` in the workflow. Tesseract is overridden to 5.5.4.
- Go 1.27.3: `Dockerfile.dev`, workflow `GO_VERSION`.
- Linux glibc floor 2.35 (FR-OCR-04): the build base is `ubuntu:22.04` and the runners are `ubuntu-22.04*`. Don't move them to a newer base.

## Code style in this repo

Keep diffs consistent with what's here:

- Go 1.27 idioms already used: `sync.WaitGroup.Go`, `for range n`, built-in `min`/`max`.
- One short doc comment per identifier, explaining *why* or the unit. No narrating comments.
- No new dependencies. Standard library plus `onnxruntime_go` and Tesseract's C API only.
- Errors to the client are answers (`bad_image` / `internal` + short message). Details go to stderr.
- Tests are named `TestFROCR0x_*` after the spec item they cover. e2e tests sit behind the `e2e` build tag.
- `testdata/synthetic/` comes from `gen.py` (seed 7). Regenerate it rather than editing the PNGs or `gt.json` by hand.
