//go:build windows

package tun

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Keep the upstream signed DLLs unchanged, including their redistribution license.
//
//go:embed assets/wintun-0.14.1.zip
var wintunArchive []byte

var wintunMu sync.Mutex

// Prepare installs bundled Windows TUN support before xray creates the adapter.
func Prepare(logf func(string, ...any)) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable for Wintun: %w", err)
	}
	return installWintun(filepath.Dir(executable), runtime.GOARCH, logf)
}

func installWintun(dir, arch string, logf func(string, ...any)) error {
	wintunMu.Lock()
	defer wintunMu.Unlock()
	output := filepath.Join(dir, "wintun.dll")
	if info, err := os.Stat(output); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Wintun path is not a file: %s", output)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check Wintun: %w", err)
	}
	if arch == "386" {
		arch = "x86"
	}
	archive, err := zip.NewReader(bytes.NewReader(wintunArchive), int64(len(wintunArchive)))
	if err != nil {
		return fmt.Errorf("open bundled Wintun: %w", err)
	}
	dll, err := archive.Open("wintun/bin/" + arch + "/wintun.dll")
	if err != nil {
		return fmt.Errorf("open bundled Wintun for %s: %w", arch, err)
	}
	defer dll.Close()
	if logf != nil {
		logf("TUN: installing bundled Wintun 0.14.1")
	}
	temporary, err := os.CreateTemp(dir, ".wintun-*.tmp")
	if err != nil {
		return fmt.Errorf("extract Wintun beside executable (folder must be writable): %w", err)
	}
	defer os.Remove(temporary.Name())
	_, copyErr := io.Copy(temporary, dll)
	closeErr := temporary.Close()
	if copyErr != nil {
		return fmt.Errorf("extract Wintun: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("save Wintun: %w", closeErr)
	}
	if err := os.Rename(temporary.Name(), output); err != nil {
		return fmt.Errorf("install Wintun: %w", err)
	}
	if logf != nil {
		logf("TUN: Wintun ready")
	}
	return nil
}
