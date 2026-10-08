package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// --- live scan dashboard ---

// liveScan renders an in-place dashboard while the worker pool runs: overall
// progress, throughput, what each worker is doing, and running counters.
type liveScan struct {
	mu      sync.Mutex
	total   int
	done    int
	current []string // file each worker is on ("" = idle)
	flagged int
	warn    int
	start   time.Time
	drawn   int // lines drawn by the previous frame
	stop    chan struct{}
	wg      sync.WaitGroup
}

func newLiveScan(total int) *liveScan {
	return &liveScan{
		total:   total,
		current: make([]string, scanWorkers),
		start:   time.Now(),
		stop:    make(chan struct{}),
	}
}

func (l *liveScan) hooks() ScanHooks {
	return ScanHooks{
		Start: func(w int, name string) {
			l.mu.Lock()
			l.current[w] = name
			l.mu.Unlock()
		},
		Done: func(w int, r ModResult) {
			l.mu.Lock()
			l.done++
			l.current[w] = ""
			switch r.Verdict {
			case VerdictFlagged:
				l.flagged++
			case VerdictWarn:
				l.warn++
			}
			l.mu.Unlock()
		},
	}
}

// run starts the redraw loop (console only).
func (l *liveScan) run() {
	if !isConsole {
		return
	}
	hideCursor()
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		t := time.NewTicker(70 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-t.C:
				l.draw(false)
			}
		}
	}()
}

// finish stops the loop and draws the final frame.
func (l *liveScan) finish() {
	if isConsole {
		close(l.stop)
		l.wg.Wait()
		l.draw(true)
		showCursor()
		return
	}
	el := time.Since(l.start)
	okLine(fmt.Sprintf("Inspected %d jars with %d workers in %s", l.total, scanWorkers, el.Round(time.Millisecond)))
}

func (l *liveScan) draw(final bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	el := time.Since(l.start)
	ratio := 1.0
	if l.total > 0 {
		ratio = float64(l.done) / float64(l.total)
	}
	rate := 0.0
	if s := el.Seconds(); s > 0 {
		rate = float64(l.done) / s
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("  %s  %s%3d%%%s   %s%d/%d%s   %s%.0f jars/s · %.1fs%s",
		gradientBar(ratio, 34),
		cBold+cWhite, int(ratio*100), cReset,
		cWhite, l.done, l.total, cReset,
		cGray, rate, el.Seconds(), cReset))

	for i, cur := range l.current {
		branch := "├─"
		if i == len(l.current)-1 {
			branch = "└─"
		}
		var status string
		switch {
		case final:
			status = cGreen + "✓ done" + cReset
		case cur == "":
			status = cDark + "· idle" + cReset
		default:
			status = cLilac + spinFrame(l.start, i*3) + cReset + " " + cWhite + trunc(strings.TrimSuffix(cur, ".jar"), 44) + cReset
		}
		lines = append(lines, fmt.Sprintf("  %s%s%s %sworker %d%s   %s", cDark, branch, cReset, cGray, i+1, cReset, status))
	}

	clean := l.done - l.flagged - l.warn
	lines = append(lines, fmt.Sprintf("  %s   %s%d flagged%s   %s%d warnings%s   %s%d clean so far%s",
		"", cRed, l.flagged, cReset, cAmber, l.warn, cReset, cGreen, clean, cReset))

	if l.drawn > 0 {
		fmt.Printf("\x1b[%dA", l.drawn)
	}
	for _, ln := range lines {
		fmt.Print("\x1b[2K" + ln + "\n")
	}
	l.drawn = len(lines)
}

// runWithSpinner runs fn while showing an animated spinner and label, then
// replaces it with a check mark and the elapsed time.
func runWithSpinner(label string, fn func()) {
	start := time.Now()
	if !isConsole {
		fn()
		okLine(label + cDim + fmt.Sprintf("  %s", time.Since(start).Round(time.Millisecond)) + cReset)
		return
	}
	hideCursor()
	defer showCursor()
	done := make(chan struct{})
	go func() { fn(); close(done) }()
	t := time.NewTicker(80 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-done:
			fmt.Printf("\r\x1b[2K  %s✓%s  %s%s  %s%s\n", cGreen, cReset, label, cDim, time.Since(start).Round(time.Millisecond), cReset)
			return
		case <-t.C:
			fmt.Printf("\r\x1b[2K  %s%s%s  %s%s%s", cLilac, spinFrame(start, 0), cReset, cWhite, label, cReset)
		}
	}
}

// --- discovery ---

// tildePath shortens the user's home directory prefix to "~".
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && len(p) >= len(home) && strings.EqualFold(p[:len(home)], home) {
		return "~" + p[len(home):]
	}
	return p
}

func printInstances(instances []Instance) {
	if len(instances) == 0 {
		warnLine("No Minecraft mod folders found on this machine.")
		info("Checked default locations for 30+ launchers (OneClient, Prism, Modrinth,")
		info("CurseForge, MultiMC, Lunar, ...) and swept every local drive.")
		return
	}
	okLine(fmt.Sprintf("%s%d%s mod folder(s) found", cBold+cWhite, len(instances), cReset))
	for _, inst := range instances {
		fmt.Printf("     %s◆%s %s %s %s\n",
			cLilac, cReset,
			padRight(cLav+trunc(inst.Launcher, 16)+cReset, 16),
			padRight(cWhite+trunc(inst.Name, 18)+cReset, 18),
			cDark+trunc(tildePath(inst.ModsDir), uiWidth-45)+cReset)
	}
}

// --- results ---

type instCounts struct{ total, ok, unk, warn, flag int }

func countNum(n int, color string) string {
	if n == 0 {
		return cDark + "–" + cReset
	}
	return color + fmt.Sprint(n) + cReset
}

// printInstanceTable shows a per-instance breakdown in a framed table.
func printInstanceTable(instances []Instance, mods []ModResult) {
	counts := map[string]*instCounts{}
	for _, inst := range instances {
		counts[inst.ModsDir] = &instCounts{}
	}
	for _, m := range mods {
		c := counts[m.Instance.ModsDir]
		if c == nil {
			continue
		}
		c.total++
		switch m.Verdict {
		case VerdictVerified:
			c.ok++
		case VerdictUnknown:
			c.unk++
		case VerdictWarn:
			c.warn++
		case VerdictFlagged:
			c.flag++
		}
	}

	row := func(l, n, t, o, u, w, f string) string {
		return padRight(l, 16) + "  " + padRight(n, 20) + padLeft(t, 6) + padLeft(o, 6) + padLeft(u, 6) + padLeft(w, 6) + padLeft(f, 6)
	}

	fmt.Println(boxTitleTop(cPurple, "INSTANCES"))
	fmt.Println(boxLine(cPurple, row(
		cGray+"LAUNCHER"+cReset, cGray+"INSTANCE"+cReset, cGray+"MODS"+cReset,
		cGray+"OK"+cReset, cGray+"UNK"+cReset, cGray+"WARN"+cReset, cGray+"FLAG"+cReset)))
	fmt.Println(boxLine(cPurple, cDark+strings.Repeat("─", uiWidth-4)+cReset))
	for _, inst := range instances {
		c := counts[inst.ModsDir]
		fmt.Println(boxLine(cPurple, row(
			cLav+trunc(inst.Launcher, 16)+cReset,
			cWhite+trunc(inst.Name, 20)+cReset,
			cBold+cWhite+fmt.Sprint(c.total)+cReset,
			countNum(c.ok, cGreen), countNum(c.unk, cYellow),
			countNum(c.warn, cAmber), countNum(c.flag, cRed+cBold))))
	}
	fmt.Println(boxBottom(cPurple))
}

// printFindings lists everything that needs a human look, most severe first.
func printFindings(mods []ModResult, logFlags []LogFlag) {
	var flagged, warned, unknown []ModResult
	verified := 0
	for _, m := range mods {
		switch m.Verdict {
		case VerdictFlagged:
			flagged = append(flagged, m)
		case VerdictWarn:
			warned = append(warned, m)
		case VerdictUnknown:
			unknown = append(unknown, m)
		case VerdictVerified:
			verified++
		}
	}

	where := func(m ModResult) string {
		return cDark + m.Instance.Launcher + " · " + m.Instance.Name + cReset
	}
	detail := func(reasons []string, color string) {
		for i, r := range reasons {
			branch := "├"
			if i == len(reasons)-1 {
				branch = "└"
			}
			fmt.Printf("          %s%s%s %s%s%s\n", cDark, branch, cReset, color, trunc(r, uiWidth-14), cReset)
		}
	}

	for _, m := range flagged {
		fmt.Printf("  %s  %s  %s\n", badge("FLAG", bgRed), cBold+cWhite+trunc(m.FileName, 34)+cReset, where(m))
		detail(m.Reasons, cRed)
	}
	for _, m := range warned {
		fmt.Printf("  %s  %s  %s\n", badge("WARN", bgAmber), cWhite+trunc(m.FileName, 34)+cReset, where(m))
		detail(m.Reasons, cAmber)
	}
	for _, lf := range logFlags {
		fmt.Printf("  %s  %s  %s\n", badge("LOG ", bgRed), cBold+cWhite+lf.Client+cReset, cDark+truncLeft(fmt.Sprintf("%s:%d", lf.File, lf.Line), 40)+cReset)
		detail([]string{lf.Excerpt}, cRed)
	}
	if len(flagged)+len(warned)+len(logFlags) == 0 {
		okLine(cGreen + "No known cheat signatures, obfuscation or log markers found." + cReset)
	}

	if len(unknown) > 0 {
		fmt.Println()
		// Group identical jars that sit in several instances.
		seen := map[string]int{}
		var names []string
		for _, m := range unknown {
			n := strings.TrimSuffix(m.FileName, ".jar")
			if seen[n] == 0 {
				names = append(names, n)
			}
			seen[n]++
		}
		sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
		fmt.Printf("  %s  %s%d mods not found on Modrinth%s %s(CurseForge-only or private, not proof)%s\n",
			badge(" ?? ", bgYellow), cWhite, len(unknown), cReset, cDark, cReset)
		const colW = 30
		cell := func(n string) string {
			s := cGray + "· " + trunc(n, colW-6) + cReset
			if c := seen[n]; c > 1 {
				s += cDark + fmt.Sprintf(" ×%d", c) + cReset
			}
			return padRight(s, colW)
		}
		for i := 0; i < len(names); i += 2 {
			line := "          " + cell(names[i])
			if i+1 < len(names) {
				line += "  " + cell(names[i+1])
			}
			fmt.Println(line)
		}
	}

	if verified > 0 {
		fmt.Println()
		fmt.Printf("  %s  %s%d mods%s verified against Modrinth %s(full list in result.txt)%s\n",
			badge(" OK ", bgGreen), cWhite, verified, cReset, cDark, cReset)
	}
}

// stackedBar draws one bar split into colored segments proportional to counts.
func stackedBar(width int, counts []int, colors []string) string {
	total := 0
	for _, n := range counts {
		total += n
	}
	if total == 0 {
		return cDark + strings.Repeat("█", width) + cReset
	}
	widths := make([]int, len(counts))
	used, largest := 0, -1
	for i, n := range counts {
		if n == 0 {
			continue
		}
		w := n * width / total
		if w < 1 {
			w = 1 // every non-zero category stays visible
		}
		widths[i] = w
		used += w
		if largest < 0 || w > widths[largest] {
			largest = i
		}
	}
	widths[largest] += width - used // absorb rounding into the largest segment
	var sb strings.Builder
	for i, w := range widths {
		if w > 0 {
			sb.WriteString(colors[i] + strings.Repeat("█", w))
		}
	}
	sb.WriteString(cReset)
	return sb.String()
}

// printSummary draws the final verdict panel.
func printSummary(instances, mods, verified, unknown, warn, flagged, logHits int, dur time.Duration, reportPath string) {
	var color, bg, verdict, sub string
	switch {
	case flagged > 0 || logHits > 0:
		color, bg, verdict, sub = cRed, bgRed, "SUSPICIOUS", "known cheat signature matched, review FLAG items"
	case warn > 0:
		color, bg, verdict, sub = cAmber, bgAmber, "INCONCLUSIVE", "obfuscated mods need a manual look"
	case unknown > 0:
		color, bg, verdict, sub = cYellow, bgYellow, "INCONCLUSIVE", "no cheats found, some mods unverifiable"
	case mods > 0:
		color, bg, verdict, sub = cGreen, bgGreen, "CLEAN", "every mod verified, nothing suspicious"
	default:
		color, bg, verdict, sub = cGray, bgPurple, "NOTHING SCANNED", "no mod folders were found"
	}

	legend := func(n int, c, label string) string {
		return c + "■" + cReset + " " + cWhite + fmt.Sprint(n) + cReset + " " + cGray + label + cReset
	}

	fmt.Println()
	fmt.Println(boxTitleTop(color, "VERDICT"))
	fmt.Println(boxLine(color, ""))
	fmt.Println(boxLine(color, "  "+badge(verdict, bg)+"  "+cGray+trunc(sub, uiWidth-12-len(verdict))+cReset))
	fmt.Println(boxLine(color, ""))
	fmt.Println(boxLine(color, "  "+stackedBar(uiWidth-8,
		[]int{verified, unknown, warn, flagged},
		[]string{cGreen, cYellow, cAmber, cRed})))
	fmt.Println(boxLine(color, "  "+legend(verified, cGreen, "verified")+"  "+legend(unknown, cYellow, "unknown")+"  "+
		legend(warn, cAmber, "warn")+"  "+legend(flagged, cRed, "flagged")+"  "+legend(logHits, cRed, "log hits")))
	fmt.Println(boxLine(color, ""))
	fmt.Println(boxLine(color, "  "+cGray+fmt.Sprintf("%d mods · %d instances · %d workers · %s", mods, instances, scanWorkers, dur.Round(time.Millisecond))+cReset))
	fmt.Println(boxLine(color, "  "+cGray+"report "+cReset+cLav+truncLeft(reportPath, uiWidth-14)+cReset))
	fmt.Println(boxBottom(color))
}
