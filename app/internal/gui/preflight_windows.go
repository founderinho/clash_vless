package gui

import (
	"errors"
	"unsafe"

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
