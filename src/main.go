package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	enableVT()
	banner()

	start := time.Now()
	divider("Locating launchers")
	instances := findInstances()
	if len(instances) == 0 {
		warnLine("No Minecraft mod folders found on this machine.")
		info("Checked default/Modrinth/Prism/PolyMC/MultiMC/CurseForge/GDLauncher/")
		info("ATLauncher/Technic/FTB/XMCL/Lunar/Feather/Badlion/TLauncher.")
	} else {
		okLine(fmt.Sprintf("%s%d%s mod folder(s) found", cBold, len(instances), cReset))
		for _, inst := range instances {
			info(fmt.Sprintf("%s%-18s%s %s%-18s%s %s", cLilac, inst.Launcher, cReset, cWhite, trunc(inst.Name, 18), cReset, cGray+inst.ModsDir+cReset))
		}
	}

	divider("Scanning mods")
	mods, online := scanMods(instances, func(done, total int, label string) {
		progressBar(done, total, label)
	})
	if len(mods) > 0 {
		progressBar(len(mods), len(mods), "complete")
		fmt.Println()
	}
	if !online {
		warnLine("Could not reach Modrinth — hash verification skipped (offline?).")
	}

	divider("Scanning logs")
	logFlags := scanLogs(instances)
	if len(logFlags) == 0 {
		okLine("no cheat-client markers in logs")
	}

	var verified, unknown, warn, flagged int
	for _, m := range mods {
		switch m.Verdict {
		case VerdictVerified:
			verified++
		case VerdictUnknown:
			unknown++
		case VerdictWarn:
			warn++
		case VerdictFlagged:
			flagged++
		}
	}

	divider("Results")
	printResults(mods, logFlags)

	// Framed summary panel.
	var resultColor, resultText string
	switch {
	case flagged > 0 || len(logFlags) > 0:
		resultColor, resultText = cRed, "SUSPICIOUS — review required"
	case warn > 0:
		resultColor, resultText = cYellow, "INCONCLUSIVE — obfuscated/unverifiable mods"
	case unknown > 0:
		resultColor, resultText = cYellow, "INCONCLUSIVE — some mods unverifiable"
	case len(mods) > 0:
		resultColor, resultText = cGreen, "CLEAN — all mods verified, no cheats"
	default:
		resultColor, resultText = cGray, "NOTHING SCANNED"
	}
	counts := fmt.Sprintf("%s%d ok%s   %s%d unknown%s   %s%d warn%s   %s%d flagged%s   %s%d log-hits%s",
		cGreen, verified, cReset, cYellow, unknown, cReset,
		cYellow, warn, cReset, cRed, flagged, cReset, cRed, len(logFlags), cReset)

	fmt.Println()
	fmt.Println(boxTop(resultColor))
	fmt.Println(boxLine(resultColor, cBold+cWhite+"SUMMARY"+cReset+cGray+fmt.Sprintf("   %d mods across %d instance(s)", len(mods), len(instances))+cReset))
	fmt.Println(boxLine(resultColor, counts))
	fmt.Println(boxLine(resultColor, ""))
	fmt.Println(boxLine(resultColor, cBold+resultColor+"RESULT: "+resultText+cReset))
	fmt.Println(boxBottom(resultColor))

	reportPath := writeReport(instances, mods, logFlags, verified, unknown, warn, flagged, online, time.Since(start))
	fmt.Println()
	okLine("Report written to " + cBold + cWhite + reportPath + cReset)
	info(fmt.Sprintf("Scan completed in %s", time.Since(start).Round(time.Millisecond)))

	fmt.Println()
	info(cDim + "Detects KNOWN clients, tampered/unpublished mods and heavy obfuscation.")
	info(cDim + "Private or custom-obfuscated cheats can still pass — use as one signal,")
	info(cDim + "not definitive proof.")

	fmt.Print("\n  " + cLilac + "Press Enter to close…" + cReset)
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func printResults(mods []ModResult, logFlags []LogFlag) {
	for _, m := range mods {
		tag := cDim + "(" + m.Instance.Launcher + "/" + m.Instance.Name + ")" + cReset
		switch m.Verdict {
		case VerdictFlagged:
			flagLine(m.FileName + "  " + tag)
			for _, r := range m.Reasons {
				fmt.Println("      " + cRed + "• " + r + cReset)
			}
		case VerdictWarn:
			warnLine(m.FileName + "  " + tag)
			for _, r := range m.Reasons {
				fmt.Println("      " + cYellow + "• " + r + cReset)
			}
		case VerdictUnknown:
			warnLine(m.FileName + cGray + "  not found on Modrinth (custom/unpublished?)" + cReset)
		case VerdictVerified:
			okLine(m.FileName + cGray + "  verified" + cReset)
		}
	}
	if len(logFlags) > 0 {
		fmt.Println()
		for _, lf := range logFlags {
			flagLine(fmt.Sprintf("log hit: %s in %s:%d", lf.Client, lf.File, lf.Line))
			fmt.Println("      " + cRed + lf.Excerpt + cReset)
		}
	}
}

func writeReport(instances []Instance, mods []ModResult, logFlags []LogFlag, verified, unknown, warn, flagged int, online bool, dur time.Duration) string {
	const path = "result.txt"
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...); b.WriteByte('\n') }

	w("================================================================")
	w(" EXO  -  Minecraft mod & log integrity report")
	w(" Generated: %s", time.Now().Format("2006-01-02 15:04:05 MST"))
	w(" Scan duration: %s", dur.Round(time.Millisecond))
	w(" Modrinth verification: %v", online)
	w("================================================================")
	w("")
	w("SUMMARY")
	w("  Instances scanned  : %d", len(instances))
	w("  Mods scanned       : %d", len(mods))
	w("  Verified (Modrinth): %d", verified)
	w("  Unknown            : %d", unknown)
	w("  Warnings (obf/meta): %d", warn)
	w("  FLAGGED (cheats)   : %d", flagged)
	w("  Log hits           : %d", len(logFlags))
	w("")
	switch {
	case flagged > 0 || len(logFlags) > 0:
		w("  OVERALL: SUSPICIOUS - matched a known cheat signature. Review required.")
	case warn > 0:
		w("  OVERALL: INCONCLUSIVE - obfuscated or metadata-less mods present.")
	case unknown > 0:
		w("  OVERALL: INCONCLUSIVE - some mods could not be verified on Modrinth.")
	case len(mods) > 0:
		w("  OVERALL: CLEAN (within detection limits) - all mods verified.")
	default:
		w("  OVERALL: NOTHING SCANNED - no mod folders found.")
	}
	w("")
	w("NOTE: Detects KNOWN cheat clients, tampered/unpublished mods and heavy")
	w("obfuscation. Private, custom-obfuscated or self-unloading cheats may not")
	w("be detected. Treat this as one signal, not definitive proof.")
	w("")

	w("----------------------------------------------------------------")
	w("INSTANCES")
	w("----------------------------------------------------------------")
	for _, inst := range instances {
		w("  [%s] %s", inst.Launcher, inst.Name)
		w("      mods: %s", inst.ModsDir)
	}
	w("")

	writeSection := func(title string, want Verdict) {
		any := false
		for _, m := range mods {
			if m.Verdict != want {
				continue
			}
			if !any {
				w("----------------------------------------------------------------")
				w(title)
				w("----------------------------------------------------------------")
				any = true
			}
			w("  %s", m.FileName)
			w("      instance : %s / %s", m.Instance.Launcher, m.Instance.Name)
			w("      path     : %s", m.Path)
			w("      sha1     : %s", m.SHA1)
			w("      sha512   : %s", m.SHA512)
			for _, r := range m.Reasons {
				w("      reason   : %s", r)
			}
			w("")
		}
	}
	writeSection("FLAGGED MODS (known cheat signatures)", VerdictFlagged)
	writeSection("WARNINGS (obfuscated / no metadata)", VerdictWarn)

	if len(logFlags) > 0 {
		w("----------------------------------------------------------------")
		w("LOG HITS")
		w("----------------------------------------------------------------")
		for _, lf := range logFlags {
			w("  client : %s", lf.Client)
			w("  file   : %s:%d", lf.File, lf.Line)
			w("  line   : %s", lf.Excerpt)
			w("")
		}
	}

	w("----------------------------------------------------------------")
	w("ALL MODS (full inventory)")
	w("----------------------------------------------------------------")
	for _, m := range mods {
		status := "UNKNOWN"
		switch m.Verdict {
		case VerdictVerified:
			status = "VERIFIED"
		case VerdictWarn:
			status = "WARN"
		case VerdictFlagged:
			status = "FLAGGED"
		}
		w("  [%-8s] %s", status, m.FileName)
		w("             instance: %s / %s", m.Instance.Launcher, m.Instance.Name)
		w("             sha1    : %s", m.SHA1)
		if m.OnModrinth {
			w("             modrinth: project %s", m.ModrinthProject)
		} else {
			w("             modrinth: not found")
		}
	}
	w("")
	w("=== end of report ===")

	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		warnLine("Failed to write result.txt: " + err.Error())
		return path + " (write failed)"
	}
	cwd, _ := os.Getwd()
	return cwd + string(os.PathSeparator) + path
}
