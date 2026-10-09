package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// screen is where all of Exo's output goes. It has two modes:
//
//   - stream (default): lines are printed straight to stdout, as a normal
//     console program does. Used in Windows Terminal, in a shell the user
//     started us from, and when output is piped.
//   - full: Exo owns a borderless console window with no scrollback. Output
//     is kept in memory and the whole window is redrawn as a frame: a custom
//     title bar (drag, minimize, close), a scrollable body with its own
//     scroll thumb, and a status bar.
type screen struct {
	mu     sync.Mutex
	full   bool
	cols   int
	rows   int
	hwnd   uintptr
	body   []string
	scroll int  // first visible body line
	follow bool // stick to the bottom while output arrives
	hover  int  // title bar button under the mouse: 0 none, 1 minimize, 2 close
	done   bool
	report string
	dirty  bool
}

var scr = &screen{follow: true}

const (
	btnMinimize = 1
	btnClose    = 2
	btnWidth    = 5 // "  ─  " / "  ×  "
)

// out prints one line (it may be empty).
func out(s string) { scr.println(s) }

// outf prints formatted text; a trailing newline is implied, and embedded
// newlines become separate lines.
func outf(format string, a ...any) {
	s := strings.TrimSuffix(fmt.Sprintf(format, a...), "\n")
	for _, l := range strings.Split(s, "\n") {
		scr.println(l)
	}
}

func (s *screen) println(line string) {
	if !s.full {
		fmt.Println(line)
		return
	}
	s.mu.Lock()
	s.body = append(s.body, line)
	s.dirty = true
	s.mu.Unlock()
}

// replaceLast swaps the last n lines for lines (n may be 0 to append). Used by
// the banner animation and the live scan dashboard.
func (s *screen) replaceLast(n int, lines []string) {
	if !s.full {
		var sb strings.Builder
		if n > 0 {
			fmt.Fprintf(&sb, "\x1b[%dA", n)
		}
		for _, l := range lines {
			sb.WriteString("\x1b[2K" + l + "\n")
		}
		fmt.Print(sb.String())
		return
	}
	s.mu.Lock()
	if n > len(s.body) {
		n = len(s.body)
	}
	s.body = append(s.body[:len(s.body)-n], lines...)
	s.dirty = true
	s.mu.Unlock()
}

// finish switches the status bar to the "done" hints.
func (s *screen) finish(reportPath string) {
	s.mu.Lock()
	s.done = true
	s.report = reportPath
	s.dirty = true
	s.mu.Unlock()
}

// --- full mode: rendering ---

func (s *screen) startRenderer() {
	fmt.Print("\x1b[?25l\x1b[?7l\x1b[2J") // hide cursor, no auto-wrap, clear
	go func() {
		t := time.NewTicker(33 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			s.mu.Lock()
			if !s.dirty {
				s.mu.Unlock()
				continue
			}
			s.dirty = false
			frame := s.frame()
			s.mu.Unlock()
			os.Stdout.WriteString(frame)
		}
	}()
}

func (s *screen) viewHeight() int { return s.rows - 2 }

func (s *screen) maxScroll() int {
	if m := len(s.body) - s.viewHeight(); m > 0 {
		return m
	}
	return 0
}

// frame renders the whole window. Caller holds s.mu.
func (s *screen) frame() string {
	viewH := s.viewHeight()
	if s.follow {
		s.scroll = s.maxScroll()
	}
	s.scroll = clamp(s.scroll, 0, s.maxScroll())

	var b strings.Builder
	fmt.Fprintf(&b, "\x1b[1;1H%s", s.titleLine())
	for i := 0; i < viewH; i++ {
		line := ""
		if idx := s.scroll + i; idx < len(s.body) {
			line = s.body[idx]
		}
		fmt.Fprintf(&b, "\x1b[%d;1H%s %s%s", i+2, cReset, padRight(line, s.cols-2), cReset)
		fmt.Fprintf(&b, "\x1b[%d;%dH%s", i+2, s.cols, s.scrollChar(i, viewH))
	}
	fmt.Fprintf(&b, "\x1b[%d;1H%s", s.rows, s.statusLine())
	return b.String()
}

func (s *screen) titleLine() string {
	const bg = "\x1b[48;5;55m"
	left := bg + "\x1b[38;5;231m\x1b[1m  ◆ EXO   " + cReset + bg + "\x1b[38;5;183mMinecraft integrity scanner" +
		"\x1b[38;5;141m   v" + appVersion
	minBtn, closeBtn := bg, bg
	if s.hover == btnMinimize {
		minBtn = "\x1b[48;5;98m"
	}
	if s.hover == btnClose {
		closeBtn = "\x1b[48;5;160m"
	}
	buttons := minBtn + "\x1b[38;5;231m  ─  " + closeBtn + "\x1b[38;5;231m  ×  " + cReset
	gap := s.cols - visLen(left) - 2*btnWidth
	if gap < 1 {
		gap = 1
	}
	return left + bg + spaces(gap) + buttons
}

func (s *screen) statusLine() string {
	const bg = "\x1b[48;5;236m"
	var left, right string
	if s.done {
		left = "\x1b[38;5;253m  ↑↓ scroll   Esc close"
		right = "\x1b[38;5;246mreport → " + truncLeft(s.report, 34) + "  "
	} else {
		left = "\x1b[38;5;253m  scanning…" + "\x1b[38;5;246m   Esc cancel"
		right = "\x1b[38;5;246mexo v" + appVersion + "  "
	}
	if len(s.body) > s.viewHeight() {
		pct := 100
		if m := s.maxScroll(); m > 0 {
			pct = s.scroll * 100 / m
		}
		right = fmt.Sprintf("\x1b[38;5;141m%3d%%   ", pct) + right
	}
	gap := s.cols - visLen(left) - visLen(right)
	if gap < 1 {
		gap = 1
	}
	return bg + left + spaces(gap) + right + cReset
}

// scrollChar draws our own thin scrollbar in the last column.
func (s *screen) scrollChar(i, viewH int) string {
	total := len(s.body)
	if total <= viewH {
		return " "
	}
	thumb := viewH * viewH / total
	if thumb < 1 {
		thumb = 1
	}
	pos := 0
	if m := s.maxScroll(); m > 0 {
		pos = s.scroll * (viewH - thumb) / m
	}
	if i >= pos && i < pos+thumb {
		return cLilac + "┃" + cReset
	}
	return cDark + "│" + cReset
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// scrollBy moves the view; reaching the bottom re-enables follow mode.
func (s *screen) scrollBy(d int) {
	s.mu.Lock()
	s.scroll = clamp(s.scroll+d, 0, s.maxScroll())
	s.follow = s.scroll >= s.maxScroll()
	s.dirty = true
	s.mu.Unlock()
}

func (s *screen) scrollTo(v int) {
	s.mu.Lock()
	s.scroll = clamp(v, 0, s.maxScroll())
	s.follow = s.scroll >= s.maxScroll()
	s.dirty = true
	s.mu.Unlock()
}

// --- full mode: input (keyboard, mouse wheel, title bar buttons, dragging) ---

var (
	procReadConsoleInput = kernel32.NewProc("ReadConsoleInputW")
	procPostMessage      = user32.NewProc("PostMessageW")
	procShowWindowCmd    = user32.NewProc("ShowWindow")
)

// inputRecord mirrors INPUT_RECORD (20 bytes): an event type and a 16-byte union.
type inputRecord struct {
	eventType uint16
	_         uint16
	event     [16]byte
}

const (
	stdInputHandle = 0xFFFFFFF6 // (DWORD)-10

	keyEvent   = 0x0001
	mouseEvent = 0x0002

	mouseMoved   = 0x0001
	mouseWheeled = 0x0004

	enableWindowInput   = 0x0008
	enableMouseInput    = 0x0010
	enableExtendedFlags = 0x0080 // with QuickEdit left out, clicks reach us instead of selecting text
)

func (s *screen) startInput() {
	in, _, _ := procGetStdHandle.Call(uintptr(uint32(stdInputHandle)))
	procSetConsoleMode.Call(in, enableExtendedFlags|enableMouseInput|enableWindowInput)
	go func() {
		var recs [16]inputRecord
		for {
			var n uint32
			r, _, _ := procReadConsoleInput.Call(in, uintptr(unsafe.Pointer(&recs[0])), uintptr(len(recs)), uintptr(unsafe.Pointer(&n)))
			if r == 0 {
				return
			}
			for i := 0; i < int(n); i++ {
				switch recs[i].eventType {
				case keyEvent:
					s.onKey(&recs[i].event)
				case mouseEvent:
					s.onMouse(&recs[i].event)
				}
			}
		}
	}()
}

func (s *screen) onKey(e *[16]byte) {
	down := *(*int32)(unsafe.Pointer(&e[0]))
	if down == 0 {
		return
	}
	vk := *(*uint16)(unsafe.Pointer(&e[6]))
	ch := *(*uint16)(unsafe.Pointer(&e[10]))
	page := s.viewHeight() - 1
	switch {
	case vk == 0x1B || ch == 3: // Esc, Ctrl+C
		s.quit()
	case vk == 0x0D: // Enter
		s.mu.Lock()
		done := s.done
		s.mu.Unlock()
		if done {
			s.quit()
		}
	case vk == 0x26: // Up
		s.scrollBy(-1)
	case vk == 0x28: // Down
		s.scrollBy(1)
	case vk == 0x21: // PgUp
		s.scrollBy(-page)
	case vk == 0x22: // PgDn
		s.scrollBy(page)
	case vk == 0x24: // Home
		s.scrollTo(0)
	case vk == 0x23: // End
		s.scrollTo(1 << 30)
	}
}

func (s *screen) buttonAt(x, y int) int {
	if y != 0 {
		return 0
	}
	switch {
	case x >= s.cols-btnWidth:
		return btnClose
	case x >= s.cols-2*btnWidth:
		return btnMinimize
	}
	return 0
}

func (s *screen) onMouse(e *[16]byte) {
	x := int(*(*int16)(unsafe.Pointer(&e[0])))
	y := int(*(*int16)(unsafe.Pointer(&e[2])))
	buttons := *(*uint32)(unsafe.Pointer(&e[4]))
	flags := *(*uint32)(unsafe.Pointer(&e[12]))

	switch {
	case flags&mouseWheeled != 0:
		if int16(buttons>>16) > 0 {
			s.scrollBy(-3)
		} else {
			s.scrollBy(3)
		}

	case flags&mouseMoved != 0:
		h := s.buttonAt(x, y)
		s.mu.Lock()
		if h != s.hover {
			s.hover = h
			s.dirty = true
		}
		s.mu.Unlock()
		if buttons&1 != 0 && x == s.cols-1 && y >= 1 && y <= s.viewHeight() {
			s.jumpScroll(y) // dragging along the scroll track
		}

	case flags == 0 && buttons&1 != 0: // left button pressed
		switch b := s.buttonAt(x, y); {
		case b == btnClose:
			s.quit()
		case b == btnMinimize:
			procShowWindowCmd.Call(s.hwnd, 6) // SW_MINIMIZE
		case y == 0:
			// Drag the borderless window by the title bar: hand the press to
			// Windows' own move loop (SC_MOVE | HTCAPTION).
			const wmSysCommand, scDragMove = 0x0112, 0xF012
			procPostMessage.Call(s.hwnd, wmSysCommand, scDragMove, 0)
		case x == s.cols-1 && y >= 1 && y <= s.viewHeight():
			s.jumpScroll(y)
		}
	}
}

// jumpScroll scrolls so the thumb lands at row y of the track.
func (s *screen) jumpScroll(y int) {
	s.mu.Lock()
	viewH, m := s.viewHeight(), s.maxScroll()
	s.mu.Unlock()
	if viewH > 1 {
		s.scrollTo((y - 1) * m / (viewH - 1))
	}
}

func (s *screen) quit() {
	fmt.Print("\x1b[?25h\x1b[?7h")
	os.Exit(0)
}
