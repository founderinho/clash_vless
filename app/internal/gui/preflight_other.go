//go:build !windows && !android && cgo

package gui

import (
	"os"
	"runtime"

	"fyne.io/fyne/v2"
)

// Preflight catches a missing display: the toolkit panics on it instead of returning.
func Preflight() error {
	if runtime.GOOS == "darwin" || os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return nil
	}
	return ErrUnavailable
}

func fitScreen(size fyne.Size) fyne.Size { return size }
