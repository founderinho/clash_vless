#!/usr/bin/env bash
# Builds dist/clashvless.apk without Gradle: go → javac → d8 → aapt2 → apksigner.
# Needs: Go, a JDK, $ANDROID_HOME with platforms;android-34 + build-tools;34.0.0, and ideally an NDK
# (cgo: system resolver + the 32-bit armeabi-v7a build).
set -euo pipefail
cd "$(dirname "$0")"
ROOT=$(cd .. && pwd)
if [ -z "${JAVA_HOME:-}" ]; then
  for JDK in "$HOME/.jdks"/jdk-17*; do
    if [ -f "$JDK/bin/javac.exe" ] || [ -x "$JDK/bin/javac" ]; then
      JAVA_HOME=$JDK
      break
    fi
  done
fi
if [ -n "${JAVA_HOME:-}" ]; then
  export JAVA_HOME
  export PATH="$JAVA_HOME/bin:$PATH"
fi
SDK=${ANDROID_HOME:-$HOME/Android/Sdk}
BT=$SDK/build-tools/34.0.0
JAR=$SDK/platforms/android-34/android.jar
OUT=$ROOT/tmp/android-build
NDK=${ANDROID_NDK_HOME:-}
if [ -z "$NDK" ] && [ -d "$SDK/ndk" ]; then
  NDK=$(ls -d "$SDK"/ndk/* | sort -V | tail -1)
fi
MINSDK=24
KS=${CLASHVLESS_KEYSTORE:-$HOME/.android/clashvless.keystore}
VER=$(sed -n 's/^const Version = "\(.*\)"/\1/p' "$ROOT/app/internal/store/store.go")
CODE=$(echo "$VER" | awk -F. '{print $1*10000 + $2*100 + $3}')

rm -rf "$OUT" && mkdir -p "$OUT/classes" "$ROOT/dist"

# With the NDK both ABIs build with cgo, so Go resolves via bionic/netd (the system
# resolver, Private DNS included). Without it only a cgo-less arm64 builds.
gob() {
  mkdir -p "$OUT/apk/lib/$1"
  go -C "$ROOT/app" build -trimpath -ldflags="-s -w" -o "$OUT/apk/lib/$1/libclashvless.so" .
}
if [ -n "$NDK" ]; then
  TC="$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin"
  CC="$TC/aarch64-linux-android$MINSDK-clang" GOOS=android GOARCH=arm64 CGO_ENABLED=1 gob arm64-v8a
  CC="$TC/armv7a-linux-androideabi$MINSDK-clang" GOOS=android GOARCH=arm GOARM=7 CGO_ENABLED=1 gob armeabi-v7a
else
  echo "no NDK — cgo-less arm64 only (Go's own DNS, no armeabi-v7a)"
  GOOS=android GOARCH=arm64 CGO_ENABLED=0 gob arm64-v8a
fi

mapfile -t SOURCES < <(find src -name '*.java')
javac -nowarn --release 17 -classpath "$JAR" -d "$OUT/classes" "${SOURCES[@]}"
mapfile -t CLASSES < <(find "$OUT/classes" -name '*.class')
java -cp "$BT/lib/d8.jar" com.android.tools.r8.D8 --release --min-api $MINSDK --lib "$JAR" --output "$OUT/apk" "${CLASSES[@]}"

mkdir -p "$OUT/res/drawable-nodpi" "$OUT/apk/assets/branding/png"
cp "$ROOT/app/branding/png/logo.png" "$OUT/res/drawable-nodpi/clashvless_logo.png"
cp "$ROOT/app/branding/tokens.json" "$OUT/apk/assets/branding/tokens.json"
cp "$ROOT/app/branding/png/"*.png "$OUT/apk/assets/branding/png/"
"$BT/aapt2" compile --dir "$OUT/res" -o "$OUT/res.zip"
"$BT/aapt2" link -I "$JAR" --manifest AndroidManifest.xml \
  --min-sdk-version $MINSDK --target-sdk-version 34 --version-name "$VER" --version-code "$CODE" \
  -o "$OUT/base.apk" "$OUT/res.zip"
(cd "$OUT/apk" && zip -qr "$OUT/base.apk" classes.dex lib assets)
"$BT/zipalign" -f -p 4 "$OUT/base.apk" "$OUT/aligned.apk"

if [ ! -f "$KS" ]; then
  mkdir -p "$(dirname "$KS")"
  keytool -genkeypair -keystore "$KS" -storepass android -keypass android -alias clashvless \
    -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=clashvless" >/dev/null
fi
java -cp "$BT/lib/apksigner.jar" com.android.apksigner.ApkSignerTool sign \
  --ks "$KS" --ks-pass pass:android --out "$ROOT/dist/clashvless.apk" "$OUT/aligned.apk"
echo "built $ROOT/dist/clashvless.apk ($VER)"
