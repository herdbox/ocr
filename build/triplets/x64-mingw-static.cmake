# vcpkg's community x64-mingw-static, plus _USE_MATH_DEFINES: Tesseract builds
# with -std=c++20, and MinGW's math.h hides M_PI in strict mode without it.
set(VCPKG_TARGET_ARCHITECTURE x64)
set(VCPKG_CRT_LINKAGE dynamic)
set(VCPKG_LIBRARY_LINKAGE static)
set(VCPKG_ENV_PASSTHROUGH_UNTRACKED PATH)

set(VCPKG_CMAKE_SYSTEM_NAME MinGW)
set(VCPKG_C_FLAGS "-D_USE_MATH_DEFINES")
set(VCPKG_CXX_FLAGS "-D_USE_MATH_DEFINES")
