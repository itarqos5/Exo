package main

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// cornerRadius matches the CSS border-radius of .app.
const cornerRadius = 18

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	gdi32                  = syscall.NewLazyDLL("gdi32.dll")
	dwmapi                 = syscall.NewLazyDLL("dwmapi.dll")
	procEnumWindows        = user32.NewProc("EnumWindows")
	procGetWindowThreadPID = user32.NewProc("GetWindowThreadProcessId")
	procGetClassName       = user32.NewProc("GetClassNameW")
	procGetWindowRect      = user32.NewProc("GetWindowRect")
	procSetWindowRgn       = user32.NewProc("SetWindowRgn")
	procGetDpiForWindow    = user32.NewProc("GetDpiForWindow")
	procCreateRoundRectRgn = gdi32.NewProc("CreateRoundRectRgn")
	procDwmSetWindowAttrib = dwmapi.NewProc("DwmSetWindowAttribute")
)

type winRect struct{ Left, Top, Right, Bottom int32 }

// mainWindow finds this process's Wails window.
func mainWindow() uintptr {
	pid := uint32(os.Getpid())
	var found uintptr
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var owner uint32
		procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			return 1
		}
		var cls [64]uint16
		procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&cls[0])), uintptr(len(cls)))
		if syscall.UTF16ToString(cls[:]) == "wailsWindow" {
			found = hwnd
			return 0 // stop
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return found
}

// roundCorners gives the frameless window rounded corners. Windows 11 does it
// natively (with anti-aliasing and a shadow); on Windows 10 the window is
// clipped to a rounded region, re-applied after every resize.
func roundCorners() {
	hwnd := mainWindow()
	if hwnd == 0 {
		return
	}
	if windows.RtlGetVersion().BuildNumber >= 22000 {
		pref := int32(2) // DWMWCP_ROUND
		procDwmSetWindowAttrib.Call(hwnd, 33, uintptr(unsafe.Pointer(&pref)), 4)
		return
	}
	var r winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	radius := int32(cornerRadius)
	if dpi, _, _ := procGetDpiForWindow.Call(hwnd); dpi > 0 {
		radius = radius * int32(dpi) / 96 // the window is in physical pixels
	}
	rgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(r.Right-r.Left+1), uintptr(r.Bottom-r.Top+1), uintptr(2*radius), uintptr(2*radius))
	if rgn != 0 {
		procSetWindowRgn.Call(hwnd, rgn, 1) // the system owns rgn afterwards
	}
}
