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
	setTitle("Exo · Minecraft integrity scanner")
	clearScreen()
	banner()

	start := time.Now()

	section(1, "Discover")
	var instances []Instance
	runWithSpinner("Searching launcher folders on all drives", func() {
		instances = findInstances()
	})
	printInstances(instances)

	section(2, "Inspect")
	refs := collectJars(instances)
	var mods []ModResult
	if len(refs) == 0 {
		info("No mod jars to inspect.")
	} else {
		live := newLiveScan(len(refs))
		live.run()
		mods = inspectAll(refs, live.hooks())
		live.finish()
	}

	section(3, "Verify")
	online := true
	if len(mods) > 0 {
		runWithSpinner(fmt.Sprintf("Checking %d hashes against Modrinth", len(mods)), func() {
			online = verifyModrinth(mods)
		})
		if !online {
			warnLine("Could not reach Modrinth — hash verification skipped (offline?).")
		}
	}
	var logFlags []LogFlag
	runWithSpinner("Scanning launcher logs for cheat-client markers", func() {
		logFlags = scanLogs(instances)
	})

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

	section(4, "Results")
	if len(instances) > 0 {
		printInstanceTable(instances, mods)
		fmt.Println()
	}
	printFindings(mods, logFlags)

	dur := time.Since(start)
	reportPath := writeReport(instances, mods, logFlags, verified, unknown, warn, flagged, online, dur)
	printSummary(len(instances), len(mods), verified, unknown, warn, flagged, len(logFlags), dur, reportPath)

	fmt.Println()
	fmt.Println("  " + cDark + "Exo detects KNOWN clients, tampered/unpublished mods and heavy obfuscation." + cReset)
	fmt.Println("  " + cDark + "Private or custom-obfuscated cheats can still pass — one signal, not proof." + cReset)

	fmt.Print("\n  " + cLilac + "›" + cReset + " " + cGray + "Press Enter to close" + cReset + " ")
	showCursor()
	bufio.NewReader(os.Stdin).ReadString('\n')
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
