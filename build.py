#!/usr/bin/env python3
"""Build the desktop application, and the Android APK when its tools are present."""

import argparse
import logging
import os
import subprocess
import re
import shutil
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
APP = ROOT / "app"
ANDROID = ROOT / "android"
DIST = ROOT / "dist"


def run_build(command):
    """Update package progress in place and retain compiler diagnostics."""
    process = subprocess.Popen(
        command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        text=True, errors="replace", bufsize=1,
    )
    diagnostics = []
    displayed = False
    terminal = sys.stdout.isatty()
    try:
        for line in process.stdout:
            message = line.rstrip("\r\n")
            # Go's -v output consists of import paths, one per line.
            if re.fullmatch(r"[A-Za-z0-9_./+~@-]+", message):
                if terminal:
                    width = shutil.get_terminal_size(fallback=(80, 24)).columns
                    progress = "  " + message
                    progress = progress[:max(0, width - 1)]
                    print("\r" + progress.ljust(max(0, width - 1)), end="", flush=True)
                    displayed = True
            else:
                diagnostics.append(line)
        code = process.wait()
    except KeyboardInterrupt:
        process.terminate()
        process.wait()
        raise
    finally:
        if displayed:
            width = shutil.get_terminal_size(fallback=(80, 24)).columns
            print("\r" + " " * max(0, width - 1) + "\r", end="", flush=True)
        process.stdout.close()
    if diagnostics:
        print("".join(diagnostics), end="", file=sys.stderr, flush=True)
    if code:
        raise subprocess.CalledProcessError(code, command)


def ensure_c_compiler():
    """Ensure a C compiler is available for CGO builds on Windows."""
    if sys.platform != "win32":
        return

    # If gcc or clang is already available, ensure CGO_ENABLED is 1 unless explicitly set.
    if shutil.which("gcc") or shutil.which("clang"):
        if "CGO_ENABLED" not in os.environ:
            os.environ["CGO_ENABLED"] = "1"
        return

    # Check common MinGW / MSYS2 / Clang locations on Windows.
    candidates = [
        Path(r"C:\msys64\mingw64\bin"),
        Path(r"C:\msys64\ucrt64\bin"),
        Path(r"C:\msys64\clang64\bin"),
        Path(r"C:\msys64\usr\bin"),
        Path(r"C:\mingw64\bin"),
        Path(r"C:\mingw\bin"),
        Path(r"C:\TDM-GCC-64\bin"),
        Path(r"C:\Program Files\LLVM\bin"),
    ]
    for pf in (os.environ.get("ProgramFiles", r"C:\Program Files"),
               os.environ.get("ProgramFiles(x86)", r"C:\Program Files (x86)")):
        pf_path = Path(pf)
        if pf_path.is_dir():
            candidates.extend(pf_path.glob("mingw-w64/*/bin"))
            candidates.extend(pf_path.glob("LLVM/bin"))

    for candidate in candidates:
        if candidate.is_dir() and ((candidate / "gcc.exe").is_file() or (candidate / "clang.exe").is_file()):
            logging.info("Found C compiler at %s; adding to PATH", candidate)
            os.environ["PATH"] = str(candidate) + os.pathsep + os.environ.get("PATH", "")
            if "CGO_ENABLED" not in os.environ:
                os.environ["CGO_ENABLED"] = "1"
            if (candidate / "gcc.exe").is_file():
                os.environ.setdefault("CC", "gcc")
            elif (candidate / "clang.exe").is_file():
                os.environ.setdefault("CC", "clang")
            return


def android_sdk():
    """The first SDK with the platform and build-tools android/build.sh pins."""
    candidates = [os.environ.get("ANDROID_HOME"), os.environ.get("ANDROID_SDK_ROOT")]
    home = Path.home()
    candidates += [home / "Android" / "Sdk", home / "Library" / "Android" / "sdk"]
    if os.environ.get("LOCALAPPDATA"):
        candidates.append(Path(os.environ["LOCALAPPDATA"]) / "Android" / "Sdk")
    for candidate in candidates:
        if not candidate:
            continue
        sdk = Path(candidate)
        if (sdk / "platforms" / "android-34" / "android.jar").is_file() and (sdk / "build-tools" / "35.0.0").is_dir():
            return sdk
    return None


def java_major(javac):
    try:
        out = subprocess.run([str(javac), "-version"], capture_output=True, text=True, timeout=60)
    except (OSError, subprocess.TimeoutExpired):
        return 0
    match = re.search(r"javac (\d+)", out.stdout + out.stderr)
    return int(match.group(1)) if match else 0


def android_jdk():
    """A JDK 17+ as (JAVA_HOME or None for the one on PATH)."""
    exe = "javac.exe" if sys.platform == "win32" else "javac"
    homes = [os.environ.get("JAVA_HOME")] + sorted(str(p) for p in (Path.home() / ".jdks").glob("jdk-*"))
    for home in homes:
        if home and java_major(Path(home) / "bin" / exe) >= 17:
            return True, home
    javac = shutil.which("javac")
    if javac and java_major(javac) >= 17:
        return True, None
    return False, None


def find_bash():
    """bash for android/build.sh; on Windows that is Git or MSYS2 bash, never WSL's launcher."""
    if sys.platform != "win32":
        return shutil.which("bash")
    candidates = [Path(os.environ.get(var, "")) / "Git" / "bin" / "bash.exe" for var in ("ProgramFiles", "ProgramW6432")]
    candidates += [Path(r"C:\msys64\usr\bin\bash.exe")]
    found = shutil.which("bash")
    if found and "system32" not in found.lower():
        candidates.insert(0, Path(found))
    return next((str(c) for c in candidates if c.is_file()), None)


def android_toolchain():
    """Returns (bash, env, missing) for the APK build; missing lists what is absent."""
    missing = []
    sdk = android_sdk()
    if sdk is None:
        missing.append("Android SDK with platforms;android-34 and build-tools;35.0.0 (set ANDROID_HOME)")
    has_jdk, java_home = android_jdk()
    if not has_jdk:
        missing.append("JDK 17 or newer (set JAVA_HOME)")
    bash = find_bash()
    if bash is None:
        missing.append("bash (Git for Windows or MSYS2 on Windows)")
    # build.sh picks each ABI's target and compiler; a desktop cross-build's settings must not leak in.
    env = {k: v for k, v in os.environ.items() if k not in ("GOOS", "GOARCH", "GOARM", "CGO_ENABLED", "CC", "CXX")}
    if sdk is not None:
        env["ANDROID_HOME"] = str(sdk)
    if java_home:
        env["JAVA_HOME"] = java_home
    else:
        env.pop("JAVA_HOME", None)  # an older JDK there would win inside build.sh
    if bash is not None and not missing:
        # Windows paths must become POSIX ones inside Git/MSYS2 bash.
        probe = 'for t in zip go; do command -v "$t" >/dev/null || printf "%s " "$t"; done'
        if sys.platform == "win32":
            probe = 'export PATH="$PATH:$(cygpath -u "$GOROOT_BIN")"; ' + probe
            env["GOROOT_BIN"] = str(Path(shutil.which("go") or "go").parent)
        result = subprocess.run([bash, "-c", probe], env=env, capture_output=True, text=True)
        missing += [f"{tool} (on bash's PATH)" for tool in result.stdout.split()]
    return bash, env, missing


def build_apk(bash, env):
    script = 'cd "$1" && '
    if sys.platform == "win32":
        script += ('export PATH="$PATH:$(cygpath -u "$GOROOT_BIN")"; '
                   'export ANDROID_HOME="$(cygpath -u "$ANDROID_HOME")"; '
                   '[ -n "${JAVA_HOME:-}" ] && export JAVA_HOME="$(cygpath -u "$JAVA_HOME")"; ')
    script += "exec bash ./build.sh"
    subprocess.run([bash, "-c", script, "build-apk", str(ANDROID)], env=env, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    apk = parser.add_mutually_exclusive_group()
    apk.add_argument("--apk", action="store_true", help="fail if the Android APK cannot be built")
    apk.add_argument("--no-apk", action="store_true", help="skip the Android APK")
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s", datefmt="%H:%M:%S", stream=sys.stdout)
    try:
        ensure_c_compiler()
        logging.info("Go configuration (from %s)", APP)
        subprocess.run(["go", "version"], cwd=APP, check=True)
        env = subprocess.run(
            ["go", "env", "GOOS", "GOARCH", "CGO_ENABLED", "CC", "GOFLAGS", "GOROOT", "GOPATH", "GOVERSION"],
            cwd=APP, check=True, capture_output=True, text=True,
        )
        keys = ("GOOS", "GOARCH", "CGO_ENABLED", "CC", "GOFLAGS", "GOROOT", "GOPATH", "GOVERSION")
        values = dict(zip(keys, env.stdout.splitlines()))
        for key in keys:
            logging.info("  %s=%s", key, values.get(key, ""))
        if values.get("GOOS") == "windows" and values.get("CGO_ENABLED") != "1":
            logging.warning(
                "A C compiler (e.g. MinGW-w64 gcc) is required to build clashvless.exe with GUI support. "
                "Building with CGO_ENABLED=0 will produce a binary without GUI."
            )
        DIST.mkdir(parents=True, exist_ok=True)
        builds = [("clashvless", [])]
        if values.get("GOOS") == "windows":
            builds = [("clashvless.exe", ["-ldflags=-H=windowsgui"]), ("clashvless-cli.exe", [])]
        for name, flags in builds:
            output = DIST / name
            command = ["go", "-C", str(APP), "build", "-v", *flags, "-o", str(output), "."]
            logging.info("Building %s", output)
            run_build(command)
            logging.info("Built %s", output)
        if values.get("GOOS") == "windows":
            logging.info("Wintun is bundled in both executables; no separate download is needed")
        if args.no_apk:
            return 0
        bash, apk_env, missing = android_toolchain()
        if missing:
            if args.apk:
                logging.error("Cannot build the APK; missing: %s", "; ".join(missing))
                return 1
            logging.info("Skipping the Android APK; missing: %s", "; ".join(missing))
            return 0
        logging.info("Building %s with the SDK at %s", DIST / "clashvless.apk", apk_env["ANDROID_HOME"])
        build_apk(bash, apk_env)
        logging.info("Built %s", DIST / "clashvless.apk")
        return 0
    except (OSError, subprocess.CalledProcessError, ValueError) as exc:
        logging.error("Build failed: %s", exc)
        return exc.returncode if isinstance(exc, subprocess.CalledProcessError) and exc.returncode > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
