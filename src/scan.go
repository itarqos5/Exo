package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

type Verdict int

const (
	VerdictVerified Verdict = iota // matched a known-good Modrinth file
	VerdictUnknown                 // not on Modrinth, no cheat signature
	VerdictWarn                    // suspicious (e.g. obfuscated, no metadata)
	VerdictFlagged                 // matched a cheat signature
)

// ModResult is the outcome for a single mod jar.
type ModResult struct {
	Instance        Instance
	Path            string
	FileName        string
	Size            int64
	SHA1            string
	SHA512          string
	Verdict         Verdict
	Reasons         []string // why it was flagged / warned
	OnModrinth      bool
	ModrinthProject string
}

// LogFlag is a suspicious line found in a log file.
type LogFlag struct {
	Instance Instance
	File     string
	Line     int
	Client   string
	Excerpt  string
}

type jarRef struct {
	inst Instance
	path string
}

// scanMods hashes every jar, inspects its contents for cheat signatures and
// obfuscation, then classifies it. progress is called after each jar.
func scanMods(instances []Instance, progress func(done, total int, label string)) ([]ModResult, bool) {
	var refs []jarRef
	for _, inst := range instances {
		for _, jar := range listJars(inst.ModsDir) {
			refs = append(refs, jarRef{inst, jar})
		}
	}

	var results []ModResult
	var allSHA1 []string
	total := len(refs)
	for i, ref := range refs {
		res := inspectJar(ref.inst, ref.path)
		results = append(results, res)
		if res.SHA1 != "" {
			allSHA1 = append(allSHA1, res.SHA1)
		}
		if progress != nil {
			progress(i+1, total, res.FileName)
		}
	}

	// Verify hashes against Modrinth in one batch.
	known, online := lookupHashes(allSHA1)
	for i := range results {
		if v, ok := known[results[i].SHA1]; ok {
			results[i].OnModrinth = true
			results[i].ModrinthProject = v.ProjectID
			// A Modrinth match clears a plain "unknown" and also a mere
			// obfuscation warning (many legit mods are proguarded), but it
			// never clears a cheat-signature flag — report both so a human
			// makes the call.
			if results[i].Verdict == VerdictUnknown || results[i].Verdict == VerdictWarn {
				results[i].Verdict = VerdictVerified
			}
		}
	}
	return results, online
}

func listJars(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var jars []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".jar") || strings.HasSuffix(name, ".jar.disabled") {
			jars = append(jars, filepath.Join(dir, e.Name()))
		}
	}
	return jars
}

// inspectJar hashes the file, scans zip entry names for cheat packages, and
// checks for obfuscation / missing mod metadata.
func inspectJar(inst Instance, path string) ModResult {
	res := ModResult{
		Instance: inst,
		Path:     path,
		FileName: filepath.Base(path),
		Verdict:  VerdictUnknown,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		res.Verdict = VerdictWarn
		res.Reasons = append(res.Reasons, "could not read file: "+err.Error())
		return res
	}
	res.Size = int64(len(data))

	h1 := sha1.Sum(data)
	h5 := sha512.Sum512(data)
	res.SHA1 = hex.EncodeToString(h1[:])
	res.SHA512 = hex.EncodeToString(h5[:])

	lowerName := strings.ToLower(res.FileName)

	var entryPaths []string
	var classBaseNames []string
	hasMetadata := false
	if zr, zerr := zip.NewReader(bytes.NewReader(data), int64(len(data))); zerr == nil {
		for _, f := range zr.File {
			lname := strings.ToLower(f.Name)
			entryPaths = append(entryPaths, lname)
			switch {
			case lname == "fabric.mod.json", lname == "quilt.mod.json",
				lname == "meta-inf/mods.toml", lname == "mcmod.info",
				lname == "meta-inf/neoforge.mods.toml":
				hasMetadata = true
			}
			if strings.HasSuffix(lname, ".class") {
				b := lname[strings.LastIndex(lname, "/")+1:]
				b = strings.TrimSuffix(b, ".class")
				classBaseNames = append(classBaseNames, b)
			}
		}
	}
	joinedEntries := strings.Join(entryPaths, "\n")

	// --- cheat signature match ---
	flagged := false
	for _, sig := range cheatSignatures {
		matched := false
		for _, fh := range sig.FileHint {
			if strings.Contains(lowerName, fh) {
				res.Reasons = append(res.Reasons, sig.Name+": filename contains \""+fh+"\"")
				matched = true
				break
			}
		}
		for _, pk := range sig.Package {
			if strings.Contains(joinedEntries, pk) {
				res.Reasons = append(res.Reasons, sig.Name+": jar contains package \""+pk+"\"")
				matched = true
				break
			}
		}
		if matched {
			flagged = true
		}
	}

	// --- obfuscation heuristic ---
	obf, obfReason := looksObfuscated(classBaseNames)
	if obf {
		res.Reasons = append(res.Reasons, "obfuscation: "+obfReason)
	}
	if len(classBaseNames) > 5 && !hasMetadata {
		res.Reasons = append(res.Reasons, "no mod metadata (fabric.mod.json / mods.toml) despite containing classes")
		obf = true
	}

	switch {
	case flagged:
		res.Verdict = VerdictFlagged
	case obf:
		res.Verdict = VerdictWarn
	}
	return res
}

// looksObfuscated flags jars whose class names are mostly very short or built
// from confusable characters (l/I/1/O/0) — a common obfuscator output.
func looksObfuscated(classBaseNames []string) (bool, string) {
	const minClasses = 15
	if len(classBaseNames) < minClasses {
		return false, ""
	}
	short, confusable := 0, 0
	for _, n := range classBaseNames {
		if len(n) <= 2 {
			short++
		}
		if isConfusable(n) {
			confusable++
		}
	}
	total := float64(len(classBaseNames))
	if float64(short)/total >= 0.6 {
		return true, "most class names are 1–2 characters (likely obfuscated)"
	}
	if float64(confusable)/total >= 0.4 {
		return true, "many class names use confusable characters (l/I/1/O/0)"
	}
	return false, ""
}

func isConfusable(s string) bool {
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		switch r {
		case 'l', 'I', 'i', '1', 'O', 'o', '0':
		default:
			return false
		}
	}
	return true
}

// scanLogs searches log files for launch-time cheat-client markers.
func scanLogs(instances []Instance) []LogFlag {
	var flags []LogFlag
	seen := map[string]bool{}
	for _, inst := range instances {
		for _, dir := range inst.LogDirs {
			for _, lf := range listLogFiles(dir) {
				if seen[lf] {
					continue
				}
				seen[lf] = true
				flags = append(flags, scanOneLog(inst, lf)...)
			}
		}
	}
	return flags
}

func listLogFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".txt") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

func scanOneLog(inst Instance, path string) []LogFlag {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var flags []LogFlag
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.ToLower(raw)
		for _, sig := range cheatSignatures {
			for _, hint := range sig.LogHint {
				if strings.Contains(line, hint) {
					excerpt := strings.TrimSpace(raw)
					if len(excerpt) > 200 {
						excerpt = excerpt[:200] + "..."
					}
					flags = append(flags, LogFlag{
						Instance: inst, File: path, Line: i + 1,
						Client: sig.Name, Excerpt: excerpt,
					})
				}
			}
		}
	}
	return flags
}
