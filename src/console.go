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
	cLilac  = "\x1b[38;5;141m" // accent
	cLav    = "\x1b[38;5;183m" // light lavender
	cGreen  = "\x1b[38;5;114m" // verified
	cAmber  = "\x1b[38;5;214m" // warning
	cYellow = "\x1b[38;5;221m" // unknown
	cRed    = "\x1b[38;5;203m" // flagged
	cGray   = "\x1b[38;5;244m"
	cDark   = "\x1b[38;5;238m"
	cWhite  = "\x1b[38;5;253m"
)

// Background colors for badges.
const (
	bgRed    = "203"
	bgAmber  = "214"
	bgYellow = "221"
	bgGreen  = "114"
	bgPurple = "141"
)

const uiWidth = 72 // total width of boxes/dividers

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

// isConsole is true when stdout is a real console (not a pipe/file). Live
// redraws and animations only run in that case, so redirected output stays
// readable.
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

// --- terminal control (console only) ---

func setTitle(t string) {
	if isConsole {
		fmt.Printf("\x1b]0;%s\x07", t)
	}
}
func clearScreen() {
	if isConsole {
		fmt.Print("\x1b[2J\x1b[H")
	}
}
func hideCursor() {
	if isConsole {
		fmt.Print("\x1b[?25l")
	}
}
func showCursor() {
	if isConsole {
		fmt.Print("\x1b[?25h")
	}
}

// --- text helpers ---

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// visLen returns the visible length of s, ignoring ANSI escape sequences.
func visLen(s string) int { return len([]rune(ansiRe.ReplaceAllString(s, ""))) }

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

// truncLeft keeps the end of s (useful for paths), adding a leading ellipsis.
func truncLeft(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[len(r)-max:])
	}
	return "…" + string(r[len(r)-max+1:])
}

// padRight pads s with spaces to visible width w.
func padRight(s string, w int) string {
	if n := w - visLen(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// padLeft right-aligns s within visible width w.
func padLeft(s string, w int) string {
	if n := w - visLen(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// badge renders a solid colored chip like  FLAG  with dark text.
func badge(text, bg string) string {
	return "\x1b[48;5;" + bg + "m\x1b[38;5;16m\x1b[1m " + text + " " + cReset
}

// barGrad is the violet gradient used for progress fills, dark to light.
var barGrad = []int{98, 98, 99, 99, 135, 135, 141, 141, 147, 147, 183, 183, 189}

// gradientBar draws a sleek thin progress bar whose filled part fades from
// deep violet to lavender.
func gradientBar(ratio float64, width int) string {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio*float64(width) + 0.5)
	var sb strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			c := barGrad[i*len(barGrad)/width]
			fmt.Fprintf(&sb, "\x1b[38;5;%dm━", c)
		} else {
			if i == filled {
				sb.WriteString(cDark)
			}
			sb.WriteString("━")
		}
	}
	sb.WriteString(cReset)
	return sb.String()
}

// spinFrames is a braille spinner.
var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spinFrame(start time.Time, offset int) string {
	i := int(time.Since(start)/(80*time.Millisecond)) + offset
	return spinFrames[i%len(spinFrames)]
}

// --- box drawing (rounded) ---

func boxTop(color string) string {
	return color + "╭" + strings.Repeat("─", uiWidth-2) + "╮" + cReset
}
func boxBottom(color string) string {
	return color + "╰" + strings.Repeat("─", uiWidth-2) + "╯" + cReset
}

// boxTitleTop is a top border with an inset title:  ╭─ TITLE ─────╮
func boxTitleTop(color, title string) string {
	t := " " + title + " "
	rest := uiWidth - 3 - visLen(t)
	if rest < 0 {
		rest = 0
	}
	return color + "╭─" + cReset + cBold + cWhite + t + cReset + color + strings.Repeat("─", rest) + "╮" + cReset
}

// boxLine renders one framed line; content may contain color codes.
func boxLine(color, content string) string {
	inner := uiWidth - 4 // space between "│ " and " │"
	return color + "│ " + cReset + padRight(content, inner) + color + " │" + cReset
}

// section prints a numbered section header, e.g.  01  DISCOVER ──────────
func section(num int, title string) {
	label := fmt.Sprintf("  %s%02d%s  %s%s%s ", cLilac+cBold, num, cReset, cBold+cWhite, strings.ToUpper(title), cReset)
	dashes := uiWidth - visLen(label)
	if dashes < 0 {
		dashes = 0
	}
	fmt.Println()
	fmt.Println(label + cDark + strings.Repeat("─", dashes) + cReset)
}

func info(msg string)     { fmt.Println("     " + cGray + msg + cReset) }
func okLine(msg string)   { fmt.Println("  " + cGreen + "✓" + cReset + "  " + msg) }
func warnLine(msg string) { fmt.Println("  " + cAmber + "!" + cReset + "  " + msg) }

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
	exoDepth    = 2                // extrusion length (down-right)
	exoDepthCol = "\x1b[38;5;55m"  // dark violet for the 3D side
	exoHiCore   = "\x1b[38;5;231m" // white shimmer core
	exoHiEdge   = "\x1b[38;5;189m" // lavender shimmer edge
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

// banner prints the framed 3D EXO wordmark next to a short info panel, with
// an intro shimmer on a console.
func banner() {
	buildExo()

	// Right-hand info column, aligned to the art rows.
	side := []string{
		"",
		cBold + cWhite + "Minecraft integrity scanner" + cReset,
		cGray + "mods · hashes · signatures · logs" + cReset,
		"",
		badge("v1.0", bgPurple) + "  " + cDim + "modrinth-verified" + cReset,
		cDim + fmt.Sprintf("%d workers · offline-capable", scanWorkers) + cReset,
		"",
		"",
	}
	compose := func(art []string) []string {
		out := make([]string, len(art))
		for i, l := range art {
			s := ""
			if i < len(side) {
				s = side[i]
			}
			out[i] = boxLine(cPurple, "  "+l+"    "+s)
		}
		return out
	}

	fmt.Println()
	fmt.Println(boxTop(cPurple))
	fmt.Println(boxLine(cPurple, ""))

	static := compose(renderExo(-1))
	for _, l := range static {
		fmt.Println(l)
	}

	if isConsole {
		for hl := 0; hl <= exoW+3; hl++ {
			time.Sleep(18 * time.Millisecond)
			fmt.Printf("\x1b[%dA", exoH) // cursor up to first art row
			for _, l := range compose(renderExo(hl)) {
				fmt.Println(l)
			}
		}
		fmt.Printf("\x1b[%dA", exoH) // settle on the clean static frame
		for _, l := range static {
			fmt.Println(l)
		}
	}

	fmt.Println(boxLine(cPurple, ""))
	fmt.Println(boxBottom(cPurple))
}
