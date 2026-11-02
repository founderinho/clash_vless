#!/usr/bin/env bash
# Builds dist/clashvless.apk (arm64) without Gradle: go → javac → d8 → aapt2 → apksigner.
# Needs: Go, a JDK, $ANDROID_HOME with platforms;android-34 + build-tools;34.0.0. arm64 links
# without cgo; 32-bit armeabi-v7a needs cgo, so it's built only when an NDK is installed.
set -euo pipefail
cd "$(dirname "$0")"
ROOT=$(cd .. && pwd)
SDK=${ANDROID_HOME:-$HOME/Android/Sdk}
BT=$SDK/build-tools/34.0.0
JAR=$SDK/platforms/android-34/android.jar
OUT=$ROOT/tmp/android-build
NDK=${ANDROID_NDK_HOME:-$(ls -d "$SDK"/ndk/* 2>/dev/null | sort -V | tail -1)}
MINSDK=24
KS=${CLASHVLESS_KEYSTORE:-$HOME/.android/clashvless.keystore}
VER=$(sed -n 's/^const Version = "\(.*\)"/\1/p' "$ROOT/app/internal/store/store.go")
CODE=$(echo "$VER" | awk -F. '{print $1*10000 + $2*100 + $3}')

rm -rf "$OUT" && mkdir -p "$OUT/classes" "$OUT/apk/lib/arm64-v8a" "$ROOT/dist"

GOOS=android GOARCH=arm64 CGO_ENABLED=0 go -C "$ROOT/app" build -trimpath -ldflags="-s -w" \
  -o "$OUT/apk/lib/arm64-v8a/libclashvless.so" .
if [ -n "$NDK" ]; then
  mkdir -p "$OUT/apk/lib/armeabi-v7a"
  CC="$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin/armv7a-linux-androideabi$MINSDK-clang" \
    GOOS=android GOARCH=arm GOARM=7 CGO_ENABLED=1 go -C "$ROOT/app" build -trimpath -ldflags="-s -w" \
    -o "$OUT/apk/lib/armeabi-v7a/libclashvless.so" .
else
  echo "no NDK — skipping armeabi-v7a (32-bit) build"
fi

javac -nowarn --release 17 -classpath "$JAR" -d "$OUT/classes" $(find src -name '*.java')
"$BT/d8" --release --min-api $MINSDK --lib "$JAR" --output "$OUT/apk" $(find "$OUT/classes" -name '*.class')

"$BT/aapt2" link -I "$JAR" --manifest AndroidManifest.xml \
  --min-sdk-version $MINSDK --target-sdk-version 34 --version-name "$VER" --version-code "$CODE" \
  -o "$OUT/base.apk"
(cd "$OUT/apk" && zip -qr "$OUT/base.apk" classes.dex lib)
"$BT/zipalign" -f -p 4 "$OUT/base.apk" "$OUT/aligned.apk"

if [ ! -f "$KS" ]; then
  mkdir -p "$(dirname "$KS")"
  keytool -genkeypair -keystore "$KS" -storepass android -keypass android -alias clashvless \
    -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=clashvless" >/dev/null
fi
"$BT/apksigner" sign --ks "$KS" --ks-pass pass:android --out "$ROOT/dist/clashvless.apk" "$OUT/aligned.apk"
echo "built $ROOT/dist/clashvless.apk ($VER)"
