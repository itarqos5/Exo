package main

import (
	"fmt"
	"regexp"
	"strings"
	"syscall"
	"time"
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

// isConsole is true when stdout is a real console (not a pipe/file). We only
// animate in that case, so redirected output doesn't fill with cursor codes.
var isConsole bool

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
	isConsole = true
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

// --- 3D animated EXO wordmark ---

// Block-letter faces. Every row is the same width so the grid is rectangular
// and the extrusion lines up. Layout: E(6) gap(3) X(7) gap(3) O(7).
var exoFace = []string{
	"██████   ██   ██    █████ ",
	"██        ██ ██    ██   ██",
	"█████      ███     ██   ██",
	"█████      ███     ██   ██",
	"██        ██ ██    ██   ██",
	"██████   ██   ██    █████ ",
}

// Face gradient, light lavender at top fading to deep purple at the bottom.
var exoGrad = []string{
	"\x1b[38;5;189m", "\x1b[38;5;147m", "\x1b[38;5;141m",
	"\x1b[38;5;135m", "\x1b[38;5;99m", "\x1b[38;5;98m",
}

const (
	exoDepth     = 2                  // extrusion length (down-right)
	exoDepthCol  = "\x1b[38;5;55m"    // dark violet for the 3D side
	exoHiCore    = "\x1b[38;5;231m"   // white shimmer core
	exoHiEdge    = "\x1b[38;5;189m"   // lavender shimmer edge
)

type exoCell struct {
	kind int // 0 empty, 1 face, 2 depth
	row  int // original face row (for gradient), face cells only
}

var (
	exoGrid [][]exoCell
	exoH    int
	exoW    int
)

// buildExo lays the faces on a grid and extrudes each face cell down-right to
// create the 3D side, keeping the bright face on top.
func buildExo() {
	faceH := len(exoFace)
	faceW := 0
	for _, r := range exoFace {
		if rl := len([]rune(r)); rl > faceW {
			faceW = rl
		}
	}
	exoH = faceH + exoDepth
	exoW = faceW + exoDepth
	exoGrid = make([][]exoCell, exoH)
	for i := range exoGrid {
		exoGrid[i] = make([]exoCell, exoW)
	}
	runes := make([][]rune, faceH)
	for r := range exoFace {
		runes[r] = []rune(exoFace[r])
	}
	// Pass 1: extrusion (only onto empty cells).
	for r := 0; r < faceH; r++ {
		for c := 0; c < len(runes[r]); c++ {
			if runes[r][c] != '█' {
				continue
			}
			for k := 1; k <= exoDepth; k++ {
				rr, cc := r+k, c+k
				if rr < exoH && cc < exoW && exoGrid[rr][cc].kind == 0 {
					exoGrid[rr][cc] = exoCell{kind: 2}
				}
			}
		}
	}
	// Pass 2: bright faces on top.
	for r := 0; r < faceH; r++ {
		for c := 0; c < len(runes[r]); c++ {
			if runes[r][c] == '█' {
				exoGrid[r][c] = exoCell{kind: 1, row: r}
			}
		}
	}
}

// renderExo returns the wordmark's lines. hl is the shimmer's leading column
// (negative disables the shimmer).
func renderExo(hl int) []string {
	lines := make([]string, exoH)
	for r := 0; r < exoH; r++ {
		var sb strings.Builder
		last := ""
		put := func(color, ch string) {
			if color != last {
				sb.WriteString(color)
				last = color
			}
			sb.WriteString(ch)
		}
		for c := 0; c < exoW; c++ {
			cell := exoGrid[r][c]
			switch cell.kind {
			case 0:
				put(cReset, " ")
			case 2:
				put(exoDepthCol, "█")
			case 1:
				color := exoGrad[cell.row]
				if hl >= 0 && c <= hl && c > hl-3 {
					if c == hl {
						color = exoHiCore
					} else {
						color = exoHiEdge
					}
				}
				put(color, "█")
			}
		}
		sb.WriteString(cReset)
		lines[r] = sb.String()
	}
	return lines
}

// banner prints the framed 3D EXO wordmark, with an intro shimmer on a console.
func banner() {
	buildExo()
	fmt.Println()
	fmt.Println(boxTop(cPurple))
	fmt.Println(boxLine(cPurple, ""))

	static := renderExo(-1)
	for _, l := range static {
		fmt.Println(boxLine(cPurple, l))
	}

	if isConsole {
		for hl := 0; hl <= exoW+3; hl++ {
			time.Sleep(18 * time.Millisecond)
			fmt.Printf("\x1b[%dA", exoH) // cursor up to first art row
			for _, l := range renderExo(hl) {
				fmt.Println(boxLine(cPurple, l))
			}
		}
		fmt.Printf("\x1b[%dA", exoH) // settle on the clean static frame
		for _, l := range static {
			fmt.Println(boxLine(cPurple, l))
		}
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
	bar := cLilac + strings.Repeat("█", filled) + cGray + strings.Repeat("░", width-filled) + cReset

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
