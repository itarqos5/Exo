package main

import (
	"fmt"
	"regexp"
	"strings"
	"syscall"
	"unsafe"
)

// Purple-themed palette (256-color ANSI).
const (
	cReset  = "\x1b[0m"
	cBold   = "\x1b[1m"
	cDim    = "\x1b[2m"
	cPurple = "\x1b[38;5;135m" // primary border
	cLilac  = "\x1b[38;5;177m" // bright accent
	cMag    = "\x1b[38;5;201m" // highlight
	cGreen  = "\x1b[38;5;120m" // verified
	cYellow = "\x1b[38;5;221m" // unknown / warning
	cRed    = "\x1b[38;5;203m" // flagged
	cGray   = "\x1b[38;5;244m"
	cWhite  = "\x1b[38;5;252m"
)

const uiWidth = 66 // total width of boxes/dividers

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle   = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

const (
	stdOutputHandle              = 0xFFFFFFF5 // (DWORD)-11, zero-extended
	enableVirtualTerminalProcess = 0x0004
)

// enableVT turns on ANSI escape handling in the Windows console so our colors
// and box-drawing render instead of showing as raw escape codes.
func enableVT() {
	handle, _, _ := procGetStdHandle.Call(uintptr(uint32(stdOutputHandle)))
	if handle == 0 {
		return
	}
	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}
	procSetConsoleMode.Call(handle, uintptr(mode|enableVirtualTerminalProcess))
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// trunc shortens s to max runes, adding an ellipsis when cut.
func trunc(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// visLen returns the visible length of s, ignoring ANSI escape sequences.
func visLen(s string) int { return len([]rune(ansiRe.ReplaceAllString(s, ""))) }

// --- box drawing (rounded) ---

func boxTop(color string) string {
	return color + "╭" + strings.Repeat("─", uiWidth-2) + "╮" + cReset
}
func boxBottom(color string) string {
	return color + "╰" + strings.Repeat("─", uiWidth-2) + "╯" + cReset
}

// boxLine renders one framed line; content may contain color codes.
func boxLine(color, content string) string {
	inner := uiWidth - 4 // space between the two "│ " ... " │"
	pad := inner - visLen(content)
	if pad < 0 {
		pad = 0
	}
	return color + "│ " + cReset + content + strings.Repeat(" ", pad) + color + " │" + cReset
}

// divider prints a labeled section divider, e.g.  ▍ SCANNING MODS ───────
func divider(title string) {
	label := cLilac + "▍ " + cBold + cWhite + strings.ToUpper(title) + cReset
	used := 2 + len([]rune(title))
	dashes := uiWidth - used - 1
	if dashes < 0 {
		dashes = 0
	}
	fmt.Println()
	fmt.Println(label + " " + cPurple + strings.Repeat("─", dashes) + cReset)
}

// banner prints a framed, pure-ASCII "EXO" wordmark in the purple theme.
func banner() {
	art := []string{
		`  ______  __   __  ______`,
		` |  ____| \ \ / / |  __  |`,
		` | |__     \ V /  | |  | |`,
		` |  __|    > <    | |  | |`,
		` | |____  / . \   | |__| |`,
		` |______|/_/ \_\  |______|`,
	}
	cols := []string{cPurple, cPurple, cLilac, cLilac, cMag, cMag}
	fmt.Println()
	fmt.Println(boxTop(cPurple))
	fmt.Println(boxLine(cPurple, ""))
	for i, l := range art {
		fmt.Println(boxLine(cPurple, cols[i]+cBold+l+cReset))
	}
	fmt.Println(boxLine(cPurple, ""))
	fmt.Println(boxLine(cPurple, cGray+"  minecraft mod & log integrity scanner"+cReset))
	fmt.Println(boxLine(cPurple, cDim+"  v1.0  ·  modrinth-verified  ·  offline-capable"+cReset))
	fmt.Println(boxBottom(cPurple))
}

func step(msg string)     { fmt.Println("  " + cLilac + "›" + cReset + " " + msg) }
func info(msg string)     { fmt.Println("    " + cGray + msg + cReset) }
func okLine(msg string)   { fmt.Println("  " + cGreen + "✓" + cReset + " " + msg) }
func warnLine(msg string) { fmt.Println("  " + cYellow + "!" + cReset + " " + msg) }
func flagLine(msg string) { fmt.Println("  " + cRed + cBold + "✗" + cReset + " " + cRed + msg + cReset) }

// progressBar draws/updates a single-line bracketed progress bar in place.
func progressBar(current, total int, label string) {
	if total <= 0 {
		return
	}
	const width = 28
	ratio := float64(current) / float64(total)
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * width)
	bar := cMag + strings.Repeat("█", filled) + cGray + strings.Repeat("░", width-filled) + cReset

	label = strings.TrimSuffix(label, ".jar")
	if r := []rune(label); len(r) > 22 {
		label = string(r[:19]) + "…"
	}
	// Pad/clear the tail so shorter names don't leave stale characters behind.
	fmt.Printf("\r  %s⟦%s%s⟧%s %s%3d%%%s  %s%d/%d%s  %s%-22s%s",
		cPurple, bar, cPurple, cReset,
		cWhite, int(ratio*100), cReset,
		cGray, current, total, cReset,
		cDim, label, cReset)
}
