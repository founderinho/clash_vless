//go:build cgo

package gui

import (
	"errors"
	"unsafe"

	"fyne.io/fyne/v2"
	"golang.org/x/sys/windows"
)

const (
	pfdDrawToWindow  = 0x4
	pfdSupportOpenGL = 0x20
	pfdGenericFormat = 0x40
	pfdGenericAccel  = 0x1000
	noOpenGLMessage  = "Clashvless can't open its window because this PC's graphics driver doesn't provide OpenGL.\n\nThis is common in virtual machines without 3D acceleration. Turn on 3D acceleration or update the graphics driver.\n\nThe proxy also works without the window: run clashvless-cli.exe tui"
	mbIconError      = 0x10
)

type pixelFormatDescriptor struct {
	Size, Version uint16
	Flags         uint32
	PixelType     byte
	_             [31]byte
}

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	gdi32               = windows.NewLazySystemDLL("gdi32.dll")
	getDC               = user32.NewProc("GetDC")
	releaseDC           = user32.NewProc("ReleaseDC")
	describePixelFormat = gdi32.NewProc("DescribePixelFormat")
	getSystemMetrics    = user32.NewProc("GetSystemMetrics")
	getDpiForSystem     = user32.NewProc("GetDpiForSystem")
)

// hasOpenGL applies GLFW's own filter: a pixel format must draw to a window, support
// OpenGL and be RGBA, and Windows' software fallback doesn't count. A failed query
// reports true so the toolkit still gets to try.
func hasOpenGL() bool {
	dc, _, _ := getDC.Call(0)
	if dc == 0 {
		return true
	}
	defer releaseDC.Call(0, dc)
	var pfd pixelFormatDescriptor
	size := unsafe.Sizeof(pfd)
	count, _, _ := describePixelFormat.Call(dc, 1, size, uintptr(unsafe.Pointer(&pfd)))
	if count == 0 {
		return true
	}
	for i := uintptr(1); i <= count; i++ {
		if ok, _, _ := describePixelFormat.Call(dc, i, size, uintptr(unsafe.Pointer(&pfd))); ok == 0 {
			return true
		}
		if pfd.Flags&pfdDrawToWindow == 0 || pfd.Flags&pfdSupportOpenGL == 0 || pfd.PixelType != 0 {
			continue
		}
		if pfd.Flags&pfdGenericFormat != 0 && pfd.Flags&pfdGenericAccel == 0 {
			continue
		}
		return true
	}
	return false
}

// Preflight explains a missing OpenGL in a dialog, since the release build has no
// console and the toolkit would otherwise fail without showing anything.
func Preflight() error {
	if hasOpenGL() {
		return nil
	}
	text, _ := windows.UTF16PtrFromString(noOpenGLMessage)
	title, _ := windows.UTF16PtrFromString("Clashvless")
	_, _ = windows.MessageBox(0, text, title, mbIconError)
	return errors.New("this graphics driver doesn't provide OpenGL — use `clashvless-cli tui`")
}

// fitScreen shrinks the window to the work area. If it were taller than the screen,
// Windows would clamp it behind the toolkit's back and clicks would land off target.
func fitScreen(size fyne.Size) fyne.Size {
	const smCxFullScreen, smCyFullScreen = 16, 17
	w, _, _ := getSystemMetrics.Call(smCxFullScreen)
	h, _, _ := getSystemMetrics.Call(smCyFullScreen)
	if w == 0 || h == 0 {
		return size
	}
	scale := float32(1)
	if getDpiForSystem.Find() == nil {
		if dpi, _, _ := getDpiForSystem.Call(); dpi != 0 {
			scale = float32(dpi) / 96
		}
	}
	return fyne.NewSize(min(size.Width, float32(w)/scale), min(size.Height, float32(h)/scale))
}
