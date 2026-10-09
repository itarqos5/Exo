package main

import (
	"bufio"
	"exo/engine"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	enableVT()
	applyTheme()
	defer restoreTheme()
	if !scr.full {
		titleBar() // full-screen mode draws its own interactive title bar
	}
	banner()

	start := time.Now()

	section(1, "Discover")
	var instances []engine.Instance
	runWithSpinner("Searching launcher folders on all drives", func() {
		instances = engine.FindInstances()
	})
	printInstances(instances)

	section(2, "Inspect")
	refs := engine.CollectJars(instances)
	var mods []engine.ModResult
	if len(refs) == 0 {
		info("No mod jars to inspect.")
	} else {
		live := newLiveScan(len(refs))
		live.run()
		mods = engine.InspectAll(refs, live.hooks())
		live.finish()
	}

	section(3, "Verify")
	online := true
	if len(mods) > 0 {
		runWithSpinner(fmt.Sprintf("Checking %d hashes against Modrinth", len(mods)), func() {
			online = engine.VerifyModrinth(mods)
		})
		if !online {
			warnLine("Could not reach Modrinth — hash verification skipped (offline?).")
		}
	}
	var logFlags []engine.LogFlag
	runWithSpinner("Scanning launcher logs for cheat-client markers", func() {
		logFlags = engine.ScanLogs(instances)
	})

	var verified, unknown, warn, flagged int
	for _, m := range mods {
		switch m.Verdict {
		case engine.VerdictVerified:
			verified++
		case engine.VerdictUnknown:
			unknown++
		case engine.VerdictWarn:
			warn++
		case engine.VerdictFlagged:
			flagged++
		}
	}

	section(4, "Results")
	if len(instances) > 0 {
		printInstanceTable(instances, mods)
		out("")
	}
	printFindings(mods, logFlags)

	dur := time.Since(start)
	reportPath := "result.txt"
	if cwd, err := os.Getwd(); err == nil {
		reportPath = filepath.Join(cwd, reportPath)
	}
	if err := engine.WriteReport(reportPath, engine.NewSummary(instances, mods, logFlags, online, dur)); err != nil {
		warnLine("Failed to write result.txt: " + err.Error())
		reportPath += " (write failed)"
	}
	printSummary(len(instances), len(mods), verified, unknown, warn, flagged, len(logFlags), dur, reportPath)

	out("")
	out("  " + cDark + "Detects KNOWN clients, tampered/unpublished mods and heavy obfuscation." + cReset)
	out("  " + cDark + "Private or obfuscated cheats can still pass: one signal, not proof." + cReset)

	if scr.full {
		// The window stays open for scrolling; Esc, Enter or × closes it.
		scr.finish(reportPath)
		select {}
	}
	out("")
	statusBar(reportPath)
	showCursor()
	bufio.NewReader(os.Stdin).ReadString('\n')
}
