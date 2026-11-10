//go:build windows

package main

import "golang.org/x/sys/windows"

var (
	kernel32ProcGetConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
)

func hasConsole() bool {
	hwnd, _, _ := kernel32ProcGetConsoleWindow.Call()
	return hwnd != 0
}
