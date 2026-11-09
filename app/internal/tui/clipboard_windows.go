package tui

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
)

func readClipboard() string {
	const cfUnicodeText = 13
	runtime.LockOSThread() // the clipboard is opened per thread
	defer runtime.UnlockOSThread()
	if ok, _, _ := openClipboard.Call(0); ok == 0 {
		return ""
	}
	defer closeClipboard.Call()
	handle, _, _ := getClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return ""
	}
	locked, _, _ := globalLock.Call(handle)
	if locked == 0 {
		return ""
	}
	defer globalUnlock.Call(handle)
	return windows.UTF16PtrToString(*(**uint16)(unsafe.Pointer(&locked)))
}
