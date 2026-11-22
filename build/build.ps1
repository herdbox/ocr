# Builds the herdbox-ocr package for windows/$env:GOARCH (amd64 or arm64) into
# $env:OUT (default ..\out): llvm-mingw (cgo needs the MinGW ABI), vcpkg
# x64/arm64-mingw-static Tesseract + Leptonica, ONNX Runtime from Microsoft's
# NuGet package, the VC++ runtime DLLs onnxruntime.dll needs, app-local.
#
# Needs VCPKG_ROOT, Go, Git for Windows (bash, curl), pwsh 7, and llvm-mingw's
# bin on PATH.
# VERSION is the package version N (default 0 = dev build).
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

$here = $PSScriptRoot
$src = Split-Path $here
$arch = if ($env:GOARCH) { $env:GOARCH } else { go env GOARCH }
$out = if ($env:OUT) { $env:OUT } else { Join-Path $src 'out' }
$version = if ($env:VERSION) { $env:VERSION } else { '0' }
$installed = if ($env:VCPKG_INSTALLED) { $env:VCPKG_INSTALLED } else { Join-Path $out 'vcpkg_installed' }
$license = if ($env:LICENSE_FILE) { $env:LICENSE_FILE } else { Join-Path $src 'LICENSE' }

$OrtVersion = '1.31.1'
$OrtUrl = "https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/$OrtVersion/microsoft.ml.onnxruntime.$OrtVersion.nupkg"
$OrtSha256 = '6871ccbc97f3d5bdb4095a1dcec33aa78d56f6ed8de5980ef5fa1d9f2e7c679a'

switch ($arch) {
	'amd64' { $triplet = 'x64-mingw-static'; $rid = 'win-x64'; $cpu = 'x86_64'; $vcArch = 'x64'; $hostTriplet = 'x64-windows' }
	'arm64' { $triplet = 'arm64-mingw-static'; $rid = 'win-arm64'; $cpu = 'aarch64'; $vcArch = 'arm64'; $hostTriplet = 'arm64-windows' }
	default { throw "unsupported arch $arch" }
}
if (-not $env:VCPKG_ROOT) { throw 'set VCPKG_ROOT' }
$vcpkg = Join-Path $env:VCPKG_ROOT 'vcpkg.exe'
New-Item -ItemType Directory -Force $out | Out-Null

& $vcpkg install "--x-manifest-root=$here" "--x-install-root=$installed" "--overlay-triplets=$(Join-Path $here 'triplets')" --triplet $triplet "--host-triplet=$hostTriplet"
$lib = Join-Path $installed $triplet

# pkgconf resolves Tesseract's static link line.
$tools = Join-Path $out 'vcpkg_tools'
& $vcpkg install "pkgconf:$hostTriplet" "--x-install-root=$tools" --classic
$pkgconf = Get-ChildItem -Recurse -Filter pkgconf.exe (Join-Path $tools $hostTriplet) | Select-Object -First 1
$env:PKG_CONFIG_PATH = Join-Path $lib 'lib\pkgconfig'
$env:CGO_ENABLED = '1'
$env:CC = "$cpu-w64-mingw32-clang"
$env:CXX = "$cpu-w64-mingw32-clang++"
$env:CGO_CFLAGS = (& $pkgconf.FullName --cflags tesseract) -join ' '
$env:CGO_LDFLAGS = ((& $pkgconf.FullName --static --libs tesseract) -join ' ') + ' -lc++ -static'

$models = Join-Path $out 'models'
# Git for Windows' bash runs the same pinned download as on Linux.
bash (Join-Path $here 'fetch-models.sh').Replace('\', '/') $models.Replace('\', '/')

$nupkg = Join-Path $out "ort-$OrtVersion.nupkg"
if (-not (Test-Path $nupkg) -or (Get-FileHash $nupkg -Algorithm SHA256).Hash.ToLower() -ne $OrtSha256) {
	Invoke-WebRequest -Uri $OrtUrl -OutFile $nupkg
	$got = (Get-FileHash $nupkg -Algorithm SHA256).Hash.ToLower()
	if ($got -ne $OrtSha256) { throw "onnxruntime nupkg sha256 $got, want $OrtSha256" }
}

$pkg = Join-Path $out "pkg-windows-$arch"
if (Test-Path $pkg) { Remove-Item -Recurse -Force $pkg }
New-Item -ItemType Directory -Force (Join-Path $pkg 'tessdata'), (Join-Path $pkg 'LICENSES') | Out-Null

Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [System.IO.Compression.ZipFile]::OpenRead($nupkg)
try {
	foreach ($pair in @(
			@("runtimes/$rid/native/onnxruntime.dll", 'onnxruntime.dll'),
			@('LICENSE', 'LICENSES\onnxruntime.txt'),
			@('ThirdPartyNotices.txt', 'LICENSES\onnxruntime-third-party.txt'))) {
		$entry = $zip.GetEntry($pair[0])
		if (-not $entry) { throw "nupkg has no $($pair[0])" }
		[System.IO.Compression.ZipFileExtensions]::ExtractToFile($entry, (Join-Path $pkg $pair[1]), $true)
	}
} finally { $zip.Dispose() }

Push-Location $src
try {
	$env:GOOS = 'windows'
	go build -trimpath -ldflags '-s -w -linkmode external' -o (Join-Path $pkg 'herdbox-ocr.exe') .
} finally { Pop-Location }

# The VC++ runtime onnxruntime.dll links to, from the runner's Visual Studio.
$vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
$vs = & $vswhere -latest -products * -property installationPath
$crt = Get-ChildItem -Directory "$vs\VC\Redist\MSVC\*\$vcArch\Microsoft.VC*.CRT" | Sort-Object FullName | Select-Object -Last 1
if (-not $crt) { throw "no VC++ redist for $vcArch under $vs" }
foreach ($dll in 'vcruntime140.dll', 'vcruntime140_1.dll', 'msvcp140.dll') {
	$p = Join-Path $crt.FullName $dll
	if (Test-Path $p) { Copy-Item $p $pkg } elseif ($dll -ne 'vcruntime140_1.dll' -or $arch -eq 'amd64') { throw "missing $p" }
}

Copy-Item (Join-Path $models 'det.onnx') $pkg
Copy-Item (Join-Path $models 'tessdata\vie.traineddata') (Join-Path $pkg 'tessdata')
Copy-Item (Join-Path $lib 'share\tesseract\copyright') (Join-Path $pkg 'LICENSES\tesseract.txt')
Copy-Item (Join-Path $lib 'share\leptonica\copyright') (Join-Path $pkg 'LICENSES\leptonica.txt')
foreach ($p in 'libpng', 'libjpeg-turbo', 'tiff', 'zlib') {
	$c = Join-Path $lib "share\$p\copyright"
	if (Test-Path $c) { Copy-Item $c (Join-Path $pkg "LICENSES\$p.txt") }
}
Copy-Item (Join-Path $src 'LICENSE-models.txt') (Join-Path $pkg 'LICENSES\models.txt')
Copy-Item $license (Join-Path $pkg 'LICENSES\herdbox-ocr.txt')
Set-Content -NoNewline -Path (Join-Path $pkg 'VERSION') -Value "$version`n"

$zipFile = Join-Path $out "herdbox-ocr-$version-windows-$arch.zip"
if (Test-Path $zipFile) { Remove-Item $zipFile }
[System.IO.Compression.ZipFile]::CreateFromDirectory($pkg, $zipFile)
$zipFile
