# Sourced by build.sh and test.sh: sets arch, triplet, out and the cgo flags
# for vcpkg's static Tesseract on Linux.
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
src=$(dirname "$here")
arch=${GOARCH:-$(go env GOARCH)}
out=${OUT:-$src/out}
installed=${VCPKG_INSTALLED:-$out/vcpkg_installed}
case $arch in
amd64) triplet=x64-linux rid=linux-x64 ;;
arm64) triplet=arm64-linux rid=linux-arm64 ;;
*) echo "unsupported arch $arch" >&2; exit 1 ;;
esac
lib=$installed/$triplet
export PKG_CONFIG_PATH=$lib/lib/pkgconfig
export CGO_ENABLED=1
cgo_flags() {
	CGO_CFLAGS="$(pkg-config --cflags tesseract)"
	CGO_LDFLAGS="$(pkg-config --static --libs tesseract) -lstdc++ -lm -lpthread"
	export CGO_CFLAGS CGO_LDFLAGS
}
