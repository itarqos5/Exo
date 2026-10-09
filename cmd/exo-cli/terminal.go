package main

import (
	"exo/engine"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const appVersion = engine.Version

const windowTitle = "Exo · Integrity Scanner"

// App window size (columns x rows) when Exo owns its console window.
const (
	winCols = uiWidth + 6
	winRows = 46
)

// Theme colors as #RRGGBB (for terminal escape sequences)...
const (
	themeBG     = "#0D0A14" // deep aubergine background
	themeFG     = "#E6E1F5" // soft lavender text
	themeCursor = "#A78BFA"
)

// ...and as Win32 COLORREF values (0x00BBGGRR) for the classic console.
const (
	crBG = 0x00140A0D // #0D0A14
	crFG = 0x00F5E1E6 // #E6E1F5
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	dwmapi = syscall.NewLazyDLL("dwmapi.dll")

	procGetWindowRect = user32.NewProc("GetWindowRect")
	procGetClientRect = user32.NewProc("GetClientRect")
	procShowScrollBar = user32.NewProc("ShowScrollBar")

	procGetCurrentConsoleFont      = kernel32.NewProc("GetCurrentConsoleFontEx")
	procSetConsoleScreenBufferSize = kernel32.NewProc("SetConsoleScreenBufferSize")
	procGetWindowLongPtr           = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr           = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procSystemParametersInfo       = user32.NewProc("SystemParametersInfoW")
	procDwmSetWindowAttr           = dwmapi.NewProc("DwmSetWindowAttribute")

	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procGetCSBIEx             = kernel32.NewProc("GetConsoleScreenBufferInfoEx")
	procSetCSBIEx             = kernel32.NewProc("SetConsoleScreenBufferInfoEx")
	procGetLargestWindowSize  = kernel32.NewProc("GetLargestConsoleWindowSize")
	procSetCurrentConsoleFont = kernel32.NewProc("SetCurrentConsoleFontEx")
	procSetConsoleTitle       = kernel32.NewProc("SetConsoleTitleW")
)

type coord struct{ X, Y int16 }
type smallRect struct{ Left, Top, Right, Bottom int16 }

// consoleScreenBufferInfoEx mirrors CONSOLE_SCREEN_BUFFER_INFOEX (96 bytes).
type consoleScreenBufferInfoEx struct {
	cbSize               uint32
	dwSize               coord
	dwCursorPosition     coord
	wAttributes          uint16
	srWindow             smallRect
	dwMaximumWindowSize  coord
	wPopupAttributes     uint16
	bFullscreenSupported int32
	colorTable           [16]uint32
}

// consoleFontInfoEx mirrors CONSOLE_FONT_INFOEX (84 bytes).
type consoleFontInfoEx struct {
	cbSize     uint32
	nFont      uint32
	dwFontSize coord
	fontFamily uint32
	fontWeight uint32
	faceName   [32]uint16
}

type rect struct{ Left, Top, Right, Bottom int32 }

// inWindowsTerminal reports whether we run inside Windows Terminal.
func inWindowsTerminal() bool { return os.Getenv("WT_SESSION") != "" }

// ownsConsole is true when this process is the only one attached to its
// console, which is what happens when the .exe is double-clicked. When it is
// started from an existing shell, the shell shares the console too, and we
// leave that window's look alone.
func ownsConsole() bool {
	var ids [4]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	return n == 1
}

// themeMode records which styling was applied, so we restore the right thing.
var themeMode int

const (
	themeNone    = iota
	themeVT      // Windows Terminal: escape sequences
	themeConsole // classic console window we own: Win32 APIs
)

// applyTheme gives the window Exo's look on any PC:
//   - Classic console we own (double-clicked): borderless app window with
//     custom palette, font and size, drawn by the full-screen renderer with
//     its own title bar and scrollbar.
//   - Windows Terminal: recolor background/text/cursor with OSC 10/11/12.
//   - Console shared with a shell the user started from: left untouched.
func applyTheme() {
	setConsoleTitle(windowTitle)
	switch {
	case !isConsole:
		return
	case inWindowsTerminal():
		fmt.Printf("\x1b]11;%s\x07\x1b]10;%s\x07\x1b]12;%s\x07", themeBG, themeFG, themeCursor)
		themeMode = themeVT
	case ownsConsole():
		if styleConsoleWindow() {
			themeMode = themeConsole
			scr.startRenderer()
			scr.startInput()
			return
		}
	}
	fmt.Print("\x1b[2J\x1b[3J\x1b[H") // repaint everything in the new background
}

// restoreTheme undoes escape-sequence styling. A console window we own closes
// with the program, so it needs nothing.
func restoreTheme() {
	if themeMode == themeVT {
		fmt.Print("\x1b]110\x07\x1b]111\x07\x1b]112\x07")
	}
}

func setConsoleTitle(t string) {
	if p, err := syscall.UTF16PtrFromString(t); err == nil {
		procSetConsoleTitle.Call(uintptr(unsafe.Pointer(p)))
	}
}

// styleConsoleWindow turns the console window we own into a borderless app
// window: custom palette and font, a buffer exactly the size of the window (so
// Windows never shows scrollbars), and no title bar, buttons, icon or frame.
// Exo's full-screen renderer then draws its own title bar and scrollbar.
// It returns false if the window couldn't be set up, so the caller can fall
// back to plain streaming output.
func styleConsoleWindow() bool {
	out, _, _ := procGetStdHandle.Call(uintptr(uint32(stdOutputHandle)))
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return false
	}

	// 1. Font: Cascadia Mono when installed (renders every glyph we use),
	//    otherwise Consolas, which ships with every Windows version.
	setConsoleFont(out)

	// 2. Palette + buffer == window size in one call.
	var info consoleScreenBufferInfoEx
	info.cbSize = uint32(unsafe.Sizeof(info))
	if r, _, _ := procGetCSBIEx.Call(out, uintptr(unsafe.Pointer(&info))); r == 0 {
		return false
	}
	largest, _, _ := procGetLargestWindowSize.Call(out)
	maxCols, maxRows := int16(largest&0xFFFF), int16(largest>>16&0xFFFF)
	cols, rows := int16(winCols), int16(winRows)
	if maxCols > 0 && cols > maxCols {
		cols = maxCols
	}
	if maxRows > 0 && rows > maxRows {
		rows = maxRows
	}
	info.colorTable[0] = crBG // default background
	info.colorTable[7] = crFG // default text
	info.wAttributes = 0x07   // text 7 on background 0
	info.wPopupAttributes = 0x05
	info.dwSize = coord{cols, rows} // no scrollback: no Windows scrollbars
	info.srWindow = smallRect{0, 0, cols - 1, rows - 1}
	info.dwMaximumWindowSize = coord{cols, rows}
	if r, _, _ := procSetCSBIEx.Call(out, uintptr(unsafe.Pointer(&info))); r == 0 {
		return false
	}
	waitForStableRect(hwnd) // conhost applies font and size asynchronously

	// 3. Strip the frame, then size the window to exactly cols x rows
	//    character cells, centered on screen.
	const (
		gwlStyle   = ^uintptr(15) // -16
		gwlExStyle = ^uintptr(19) // -20

		wsCaption     = 0x00C00000
		wsThickFrame  = 0x00040000
		wsMaximizeBox = 0x00010000
		wsVScroll     = 0x00200000
		wsHScroll     = 0x00100000

		wsExDlgModalFrame = 0x00000001
		wsExWindowEdge    = 0x00000100
		wsExClientEdge    = 0x00000200
	)
	style, _, _ := procGetWindowLongPtr.Call(hwnd, gwlStyle)
	style &^= wsCaption | wsThickFrame | wsMaximizeBox | wsVScroll | wsHScroll
	procSetWindowLongPtr.Call(hwnd, gwlStyle, style)
	ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
	ex &^= wsExDlgModalFrame | wsExWindowEdge | wsExClientEdge
	procSetWindowLongPtr.Call(hwnd, gwlExStyle, ex)

	// Rounded corners on Windows 11 (ignored on Windows 10).
	round := int32(2) // DWMWCP_ROUND
	procDwmSetWindowAttr.Call(hwnd, 33, uintptr(unsafe.Pointer(&round)), 4)

	var font consoleFontInfoEx
	font.cbSize = uint32(unsafe.Sizeof(font))
	procGetCurrentConsoleFont.Call(out, 0, uintptr(unsafe.Pointer(&font)))
	cellW, cellH := int32(font.dwFontSize.X), int32(font.dwFontSize.Y)
	if cellW <= 0 || cellH <= 0 {
		var client rect // fall back to the current text area
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
		cellW, cellH = (client.Right-client.Left)/int32(cols), (client.Bottom-client.Top)/int32(rows)
	}
	placeCentered(hwnd, cellW*int32(cols), cellH*int32(rows))
	waitForStableRect(hwnd)

	// conhost decides the visible rows from the window's pixel size. Make the
	// buffer match what it settled on so no Windows scrollbar can appear.
	var check consoleScreenBufferInfoEx
	check.cbSize = uint32(unsafe.Sizeof(check))
	procGetCSBIEx.Call(out, uintptr(unsafe.Pointer(&check)))
	visCols := check.srWindow.Right - check.srWindow.Left + 1
	visRows := check.srWindow.Bottom - check.srWindow.Top + 1
	if check.dwSize.X != visCols || check.dwSize.Y != visRows {
		procSetConsoleScreenBufferSize.Call(out, uintptr(uint16(visCols))|uintptr(uint16(visRows))<<16)
	}
	const sbBoth = 3
	procShowScrollBar.Call(hwnd, sbBoth, 0)

	// Read back the real size for the renderer.
	procGetCSBIEx.Call(out, uintptr(unsafe.Pointer(&check)))
	scr.full = true
	scr.hwnd = hwnd
	scr.cols = int(check.srWindow.Right-check.srWindow.Left) + 1
	scr.rows = int(check.srWindow.Bottom-check.srWindow.Top) + 1
	return scr.cols > 20 && scr.rows > 10
}

func setConsoleFont(out uintptr) {
	faces := []string{"Consolas"}
	if fontInstalled("CascadiaMono.ttf") || fontInstalled("CascadiaCode.ttf") {
		faces = []string{"Cascadia Mono", "Cascadia Code", "Consolas"}
	}
	for _, face := range faces {
		var f consoleFontInfoEx
		f.cbSize = uint32(unsafe.Sizeof(f))
		f.dwFontSize = coord{0, 18}
		f.fontFamily = 54 // FF_MODERN | TMPF_TRUETYPE | TMPF_VECTOR
		f.fontWeight = 400
		name, _ := syscall.UTF16FromString(face)
		copy(f.faceName[:], name)
		if r, _, _ := procSetCurrentConsoleFont.Call(out, 0, uintptr(unsafe.Pointer(&f))); r != 0 {
			return
		}
	}
}

func fontInstalled(file string) bool {
	for _, dir := range []string{
		filepath.Join(os.Getenv("WINDIR"), "Fonts"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"),
	} {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			return true
		}
	}
	return false
}

// waitForStableRect waits until the window stops changing size, since conhost
// applies font and buffer changes asynchronously.
func waitForStableRect(hwnd uintptr) {
	var wr, prev rect
	stable := 0
	for i := 0; i < 40 && stable < 3; i++ {
		time.Sleep(25 * time.Millisecond)
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
		if wr == prev {
			stable++
		} else {
			stable = 0
		}
		prev = wr
	}
}

// placeCentered sizes the window to w x h and centers it on the primary
// monitor's work area, applying the new (frameless) style at the same time.
func placeCentered(hwnd uintptr, w, h int32) {
	var work rect
	const spiGetWorkArea = 0x0030
	procSystemParametersInfo.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&work)), 0)
	x := work.Left + (work.Right-work.Left-w)/2
	y := work.Top + (work.Bottom-work.Top-h)/2
	if x < work.Left {
		x = work.Left
	}
	if y < work.Top {
		y = work.Top
	}
	const swpNoZOrder, swpFrameChanged, swpShowWindow = 0x0004, 0x0020, 0x0040
	procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoZOrder|swpFrameChanged|swpShowWindow)
}

// titleBar draws the full-width header strip at the top of the app.
func titleBar() {
	left := "  ◆ EXO   " + "\x1b[38;5;183m" + "Minecraft integrity scanner"
	right := "v" + appVersion + "  "
	bar(left, right, "55", "\x1b[38;5;231m\x1b[1m")
}

// statusBar draws the footer strip with the close hint.
func statusBar(reportPath string) {
	left := "  ⏎ Enter  close" + "\x1b[38;5;246m" + "   ·   report → " + truncLeft(reportPath, 34)
	right := "exo v" + appVersion + "  "
	bar(left, right, "236", "\x1b[38;5;253m")
}

// bar renders one solid full-width strip with left and right aligned text.
func bar(left, right, bg, fg string) {
	gap := uiWidth - visLen(left) - visLen(right)
	if gap < 1 {
		gap = 1
	}
	out(fmt.Sprintf("\x1b[48;5;%sm%s%s%s%s%s", bg, fg, left, spaces(gap), fg+right, cReset))
}

func spaces(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
