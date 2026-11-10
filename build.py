#!/usr/bin/env python3
"""Build the desktop application using the active Go configuration."""

import logging
import os
import subprocess
import re
import shutil
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
APP = ROOT / "app"
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


def main():
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
        return 0
    except (OSError, subprocess.CalledProcessError, ValueError) as exc:
        logging.error("Build failed: %s", exc)
        return exc.returncode if isinstance(exc, subprocess.CalledProcessError) and exc.returncode > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
