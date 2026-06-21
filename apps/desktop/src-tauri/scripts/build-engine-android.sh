#!/bin/sh
set -e

: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to your NDK path}"

ENGINE_DIR="$(cd "$(dirname "$0")/../../../../engine" && pwd)"
OUT="${1:-/tmp/linetta-android}"
API="${ANDROID_API:-24}"
case "$(uname -s)" in
  Darwin)
    HOST_TAG="darwin-x86_64"
    if [ ! -d "$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$HOST_TAG" ] && \
       [ -d "$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/darwin-arm64" ]; then
      HOST_TAG="darwin-arm64"
    fi
    ;;
  Linux)
    HOST_TAG="linux-x86_64"
    ;;
  *)
    echo "unsupported Android NDK host: $(uname -s)" >&2
    exit 1
    ;;
esac
TC="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$HOST_TAG/bin"

build() {
  abi="$1"
  goarch="$2"
  cc="$3"
  mkdir -p "$OUT/$abi"
  cd "$ENGINE_DIR"
  CGO_ENABLED=1 GOOS=android GOARCH="$goarch" CC="$TC/$cc" \
    go build -tags mobile -buildmode=c-shared -o "$OUT/$abi/liblinetta.so" ./cmd/linetta-ffi
}

build arm64-v8a arm64 "aarch64-linux-android${API}-clang"
build armeabi-v7a arm "armv7a-linux-androideabi${API}-clang"
build x86_64 amd64 "x86_64-linux-android${API}-clang"
echo "built .so for arm64-v8a, armeabi-v7a, x86_64 under $OUT"
