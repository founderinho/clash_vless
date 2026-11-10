//go:build !cgo && windows

package gui

import (
	"golang.org/x/sys/windows"

	"clashvless/internal/control"
)

const (
	noCGOMessage = "This build of Clashvless does not contain the graphical interface because it was compiled without CGO (a C compiler like MinGW-w64 gcc was missing at build time).\n\nPlease install MinGW-w64 and rebuild using `python build.py`, or use `clashvless-cli.exe tui`."
	mbIconError  = 0x10
)

// Preflight explains missing GUI support in a dialog if running without an attached console,
// because a windowed build (-H=windowsgui) has no console to show errors.
func Preflight() error {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd == 0 {
		text, _ := windows.UTF16PtrFromString(noCGOMessage)
		title, _ := windows.UTF16PtrFromString("Clashvless")
		_, _ = windows.MessageBox(0, text, title, mbIconError)
	}
	return ErrUnavailable
}

func RunClient(*control.Client, bool) error { return ErrUnavailable }
