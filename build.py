#!/usr/bin/env python3
"""Build the desktop application using the active Go configuration."""

import logging
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
            logging.info("Wintun is bundled in both executables; no separate download is needed")
        return 0
    except (OSError, subprocess.CalledProcessError, ValueError) as exc:
        logging.error("Build failed: %s", exc)
        return exc.returncode if isinstance(exc, subprocess.CalledProcessError) and exc.returncode > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
