#!/bin/sh
set -e

ENGINE_DIR="$(cd "$(dirname "$0")/../../../../engine" && pwd)"
OUT="${1:-/tmp/linetta-ios}"
BUILD_DIR="$OUT/build"
INCLUDE_DIR="$OUT/include"
mkdir -p "$OUT/device" "$OUT/sim" "$BUILD_DIR" "$INCLUDE_DIR"

mk_wrap() {
  sdk="$1"
  minflag="$2"
  wrapper="$BUILD_DIR/clangwrap-$sdk.sh"
  cat > "$wrapper" <<EOF
#!/bin/sh
exec "\$(xcrun --sdk $sdk --find clang)" -isysroot "\$(xcrun --sdk $sdk --show-sdk-path)" -arch arm64 $minflag "\$@"
EOF
  chmod +x "$wrapper"
  echo "$wrapper"
}

DEV_CC="$(mk_wrap iphoneos "-miphoneos-version-min=13.0")"
SIM_CC="$(mk_wrap iphonesimulator "-mios-simulator-version-min=13.0")"

cd "$ENGINE_DIR"
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$DEV_CC" \
  go build -tags mobile -buildmode=c-archive -o "$OUT/device/liblinetta.a" ./cmd/linetta-ffi
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$SIM_CC" \
  go build -tags mobile -buildmode=c-archive -o "$OUT/sim/liblinetta.a" ./cmd/linetta-ffi
cp "$OUT/device/liblinetta.h" "$OUT/"
cp "$OUT/device/liblinetta.h" "$INCLUDE_DIR/"

rm -rf "$OUT/LinettaEngine.xcframework"
xcodebuild -create-xcframework \
  -library "$OUT/device/liblinetta.a" -headers "$INCLUDE_DIR" \
  -library "$OUT/sim/liblinetta.a" -headers "$INCLUDE_DIR" \
  -output "$OUT/LinettaEngine.xcframework"
echo "built: $OUT/LinettaEngine.xcframework"
