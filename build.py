#!/usr/bin/env python3
"""Build the desktop application using the active Go configuration."""

import io
import logging
import subprocess
import re
import shutil
import sys
import urllib.request
import zipfile
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


def install_wintun(goarch):
    """Install the signed upstream DLL matching the Windows build target."""
    arches = {"amd64": "amd64", "386": "x86", "arm64": "arm64", "arm": "arm"}
    if goarch not in arches:
        raise ValueError(f"Wintun does not support Windows GOARCH={goarch}")
    url = "https://www.wintun.net/builds/wintun-0.14.1.zip"
    member = f"wintun/bin/{arches[goarch]}/wintun.dll"
    logging.info("Downloading Wintun 0.14.1 for %s", goarch)
    with urllib.request.urlopen(url, timeout=60) as response:
        archive_data = response.read()
    with zipfile.ZipFile(io.BytesIO(archive_data)) as archive:
        dll = archive.read(member)
    output = DIST / "wintun.dll"
    # Replace only after the full archive has been downloaded and validated.
    import tempfile
    import os
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=DIST, delete=False) as f:
            temporary = Path(f.name)
            f.write(dll)
        os.replace(temporary, output)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)
    logging.info("Installed %s", output)


def main():
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s", datefmt="%H:%M:%S", stream=sys.stdout)
    try:
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
            install_wintun(values.get("GOARCH", ""))
        return 0
    except (OSError, subprocess.CalledProcessError, ValueError, KeyError, zipfile.BadZipFile) as exc:
        logging.error("Build failed: %s", exc)
        return exc.returncode if isinstance(exc, subprocess.CalledProcessError) and exc.returncode > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
