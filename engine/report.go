package engine

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Version is the Exo release version, shown in both apps and the report.
const Version = "1.2.0"

// Summary is the outcome of one full scan.
type Summary struct {
	Instances []Instance
	Mods      []ModResult
	LogFlags  []LogFlag
	Verified  int
	Unknown   int
	Warn      int
	Flagged   int
	Online    bool // Modrinth was reachable
	Duration  time.Duration
}

// NewSummary tallies verdicts for a finished scan.
func NewSummary(instances []Instance, mods []ModResult, logFlags []LogFlag, online bool, dur time.Duration) Summary {
	s := Summary{Instances: instances, Mods: mods, LogFlags: logFlags, Online: online, Duration: dur}
	for _, m := range mods {
		switch m.Verdict {
		case VerdictVerified:
			s.Verified++
		case VerdictUnknown:
			s.Unknown++
		case VerdictWarn:
			s.Warn++
		case VerdictFlagged:
			s.Flagged++
		}
	}
	return s
}

// Overall is the single verdict for a scan, most severe first.
type Overall string

const (
	OverallSuspicious   Overall = "suspicious"
	OverallInconclusive Overall = "inconclusive"
	OverallClean        Overall = "clean"
	OverallEmpty        Overall = "empty"
)

func (s Summary) Overall() Overall {
	switch {
	case s.Flagged > 0 || len(s.LogFlags) > 0:
		return OverallSuspicious
	case s.Warn > 0 || s.Unknown > 0:
		return OverallInconclusive
	case len(s.Mods) > 0:
		return OverallClean
	}
	return OverallEmpty
}

// WriteReport writes the full plain-text report to path.
func WriteReport(path string, s Summary) error {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...); b.WriteByte('\n') }

	w("================================================================")
	w(" EXO v%s  -  Minecraft mod & log integrity report", Version)
	w(" Generated: %s", time.Now().Format("2006-01-02 15:04:05 MST"))
	w(" Scan duration: %s", s.Duration.Round(time.Millisecond))
	w(" Modrinth verification: %v", s.Online)
	w("================================================================")
	w("")
	w("SUMMARY")
	w("  Instances scanned  : %d", len(s.Instances))
	w("  Mods scanned       : %d", len(s.Mods))
	w("  Verified (Modrinth): %d", s.Verified)
	w("  Unknown            : %d", s.Unknown)
	w("  Warnings (obf/meta): %d", s.Warn)
	w("  FLAGGED (cheats)   : %d", s.Flagged)
	w("  Log hits           : %d", len(s.LogFlags))
	w("")
	switch {
	case s.Flagged > 0 || len(s.LogFlags) > 0:
		w("  OVERALL: SUSPICIOUS - matched a known cheat signature. Review required.")
	case s.Warn > 0:
		w("  OVERALL: INCONCLUSIVE - obfuscated or metadata-less mods present.")
	case s.Unknown > 0:
		w("  OVERALL: INCONCLUSIVE - some mods could not be verified on Modrinth.")
	case len(s.Mods) > 0:
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
	for _, inst := range s.Instances {
		w("  [%s] %s", inst.Launcher, inst.Name)
		w("      mods: %s", inst.ModsDir)
	}
	w("")

	writeSection := func(title string, want Verdict) {
		header := false
		for _, m := range s.Mods {
			if m.Verdict != want {
				continue
			}
			if !header {
				w("----------------------------------------------------------------")
				w("%s", title)
				w("----------------------------------------------------------------")
				header = true
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

	if len(s.LogFlags) > 0 {
		w("----------------------------------------------------------------")
		w("LOG HITS")
		w("----------------------------------------------------------------")
		for _, lf := range s.LogFlags {
			w("  client : %s", lf.Client)
			w("  file   : %s:%d", lf.File, lf.Line)
			w("  line   : %s", lf.Excerpt)
			w("")
		}
	}

	w("----------------------------------------------------------------")
	w("ALL MODS (full inventory)")
	w("----------------------------------------------------------------")
	for _, m := range s.Mods {
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

	return os.WriteFile(path, []byte(b.String()), 0644)
}
