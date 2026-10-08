package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Instance is one Minecraft profile/instance with a mods folder and log folders.
type Instance struct {
	Launcher string
	Name     string
	ModsDir  string
	LogDirs  []string
}

// launcherRoot pairs a friendly launcher label with a directory to deep-scan
// for "mods" folders. Any env var that is empty is skipped.
type launcherRoot struct {
	Label string
	Dir   string
}

// launcherKeywords maps a lowercase folder-name fragment to a launcher label.
// Used both to spot launcher folders during the drive sweep and to label
// whatever we find there. Order matters: more specific fragments first.
var launcherKeywords = []struct{ key, label string }{
	{"oneclient", "OneClient"},
	{"elyprism", "ElyPrism Launcher"},
	{"prismlauncher", "Prism Launcher"},
	{"prism", "Prism Launcher"},
	{"polymc", "PolyMC"},
	{"ultimmc", "UltimMC"},
	{"multimc", "MultiMC"},
	{"fjord", "Fjord Launcher"},
	{"freesm", "Freesm Launcher"},
	{"modrinth", "Modrinth App"},
	{"curseforge", "CurseForge"},
	{"gdlauncher", "GDLauncher"},
	{"atlauncher", "ATLauncher"},
	{"technic", "Technic"},
	{"ftba", "FTB App"},
	{"xmcl", "XMCL"},
	{"hmcl", "HMCL"},
	{"lunarclient", "Lunar Client"},
	{"feather", "Feather Client"},
	{"badlion", "Badlion Client"},
	{"labymod", "LabyMod"},
	{"salwyrr", "Salwyrr"},
	{"sklauncher", "SKLauncher"},
	{"tlauncher", "TLauncher"},
	{"minecraft", "Minecraft"},
}

// launcherLabel guesses which launcher a path belongs to from its folder names.
func launcherLabel(path string) string {
	p := strings.ToLower(path)
	for _, k := range launcherKeywords {
		if strings.Contains(p, k.key) {
			return k.label
		}
	}
	return "Other"
}

// findInstances locates Minecraft mod folders across every launcher we know of.
// It deep-scans each launcher's root directory for folders literally named
// "mods" that contain at least one jar, so it is not limited to .minecraft.
// It first checks the launchers' default locations, then sweeps every local
// drive for launcher folders installed elsewhere (e.g. D:\Minecraft\OneClient).
func findInstances() []Instance {
	appdata := os.Getenv("APPDATA")    // Roaming
	local := os.Getenv("LOCALAPPDATA") // Local
	home, _ := os.UserHomeDir()        // %USERPROFILE%
	docs := filepath.Join(home, "Documents")

	roots := []launcherRoot{
		{"Minecraft (default)", join(appdata, ".minecraft")},
		{"OneClient", join(appdata, "OneClient")},
		{"OneClient", join(local, "OneClient")},
		{"OneClient", join(home, "OneClient")},
		{"OneClient", join(appdata, "Polyfrost", "OneClient")},
		{"Modrinth App", join(appdata, "ModrinthApp")},
		{"Modrinth (Theseus)", join(appdata, "com.modrinth.theseus")},
		{"Prism Launcher", join(appdata, "PrismLauncher")},
		{"ElyPrism Launcher", join(appdata, "ElyPrismLauncher")},
		{"Fjord Launcher", join(appdata, "FjordLauncher")},
		{"Freesm Launcher", join(appdata, "FreesmLauncher")},
		{"PolyMC", join(appdata, "PolyMC")},
		{"UltimMC", join(appdata, "UltimMC")},
		{"MultiMC", join(appdata, ".multimc")},
		{"MultiMC", join(home, "MultiMC")},
		{"MultiMC", join(docs, "MultiMC")},
		{"CurseForge", join(home, "curseforge")},
		{"CurseForge", join(docs, "CurseForge")},
		{"CurseForge (legacy)", join(docs, "Curse")},
		{"GDLauncher", join(appdata, "gdlauncher_next")},
		{"GDLauncher Carbon", join(appdata, "gdlauncher_carbon")},
		{"GDLauncher Carbon", join(local, "gdlauncher_carbon")},
		{"ATLauncher", join(home, "ATLauncher")},
		{"ATLauncher", join(appdata, "ATLauncher")},
		{"Technic", join(appdata, ".technic")},
		{"FTB App", join(home, ".ftba")},
		{"FTB App", join(local, ".ftba")},
		{"XMCL", join(appdata, "xmcl")},
		{"XMCL", join(local, "xmcl")},
		{"HMCL", join(appdata, ".hmcl")},
		{"Lunar Client", join(home, ".lunarclient")},
		{"Feather Client", join(appdata, ".feather")},
		{"Badlion Client", join(appdata, "Badlion Client")},
		{"LabyMod", join(appdata, "LabyMod")},
		{"Salwyrr", join(appdata, ".salwyrr")},
		{"TLauncher", join(appdata, ".tlauncher")},
		{"SKLauncher", join(appdata, ".minecraft_sklauncher")},
		{"Legacy Launcher", join(appdata, ".tlauncherlegacy")},
	}

	// Launchers installed outside AppData, on any local drive.
	for _, dir := range sweepDrives() {
		roots = append(roots, launcherRoot{launcherLabel(dir), dir})
	}

	var out []Instance
	seen := map[string]bool{}

	for _, r := range roots {
		if r.Dir == "" {
			continue
		}
		if st, err := os.Stat(r.Dir); err != nil || !st.IsDir() {
			continue
		}
		for _, modsDir := range findModsDirs(r.Dir, 6) {
			resolved, err := filepath.EvalSymlinks(modsDir)
			if err != nil {
				resolved = filepath.Clean(modsDir)
			}
			key := strings.ToLower(resolved)
			if seen[key] {
				continue
			}
			seen[key] = true

			parent := filepath.Dir(modsDir)
			name := filepath.Base(parent)
			// MultiMC/Prism layout: instances\<name>\(.)minecraft\mods — use <name>.
			if ln := strings.ToLower(name); ln == "minecraft" || ln == ".minecraft" {
				grand := filepath.Dir(parent)
				if strings.EqualFold(filepath.Base(filepath.Dir(grand)), "instances") {
					name = filepath.Base(grand)
				}
			}
			if name == "." || name == string(filepath.Separator) {
				name = "main"
			}
			logDirs := []string{filepath.Join(parent, "logs")}
			// OneClient: <root>\clusters\<name>\mods, logs shared in <root>\.minecraft\logs
			if grand := filepath.Dir(parent); strings.EqualFold(filepath.Base(grand), "clusters") {
				logDirs = append(logDirs, filepath.Join(filepath.Dir(grand), ".minecraft", "logs"))
			}
			label := r.Label
			if label == "Minecraft" || label == "Other" {
				// A generic drive-sweep folder: label by the actual path instead.
				label = launcherLabel(modsDir)
			}
			out = append(out, Instance{
				Launcher: label,
				Name:     name,
				ModsDir:  modsDir,
				LogDirs:  logDirs,
			})
		}
	}
	return out
}

// Windows drive types (GetDriveTypeW).
const (
	driveRemovable = 2
	driveFixed     = 3
)

var (
	procGetLogicalDrives = kernel32.NewProc("GetLogicalDrives")
	procGetDriveTypeW    = kernel32.NewProc("GetDriveTypeW")
)

// localDrives returns the roots ("C:\", "D:\", ...) of fixed and removable drives.
// Network, optical and RAM drives are skipped.
func localDrives() []string {
	mask, _, _ := procGetLogicalDrives.Call()
	var drives []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		t, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
		if t == driveFixed || t == driveRemovable {
			drives = append(drives, root)
		}
	}
	return drives
}

// sweepSkip are top-level folders never worth looking inside on a drive.
var sweepSkip = map[string]bool{
	"windows": true, "$recycle.bin": true, "system volume information": true,
	"$windows.~bt": true, "$windows.~ws": true, "recovery": true,
	"programdata": true, "perflogs": true, "msocache": true, "config.msi": true,
}

// sweepDrives looks at the first two folder levels of every local drive and
// returns folders whose names look like a Minecraft launcher, e.g.
// D:\Minecraft or G:\Games\PrismLauncher. Only those are deep-scanned
// afterwards, which keeps the sweep fast instead of walking whole drives.
func sweepDrives() []string {
	var hits []string
	for _, drive := range localDrives() {
		top, err := os.ReadDir(drive)
		if err != nil {
			continue
		}
		for _, e := range top {
			if sweepSkip[strings.ToLower(e.Name())] {
				continue
			}
			lvl1 := filepath.Join(drive, e.Name())
			if st, err := os.Stat(lvl1); err != nil || !st.IsDir() {
				continue
			}
			if isLauncherName(e.Name()) {
				hits = append(hits, launcherDirs(lvl1)...)
				continue // anything below it is covered
			}
			sub, err := os.ReadDir(lvl1)
			if err != nil {
				continue
			}
			for _, s := range sub {
				if !isLauncherName(s.Name()) {
					continue
				}
				lvl2 := filepath.Join(lvl1, s.Name())
				if st, err := os.Stat(lvl2); err == nil && st.IsDir() {
					hits = append(hits, launcherDirs(lvl2)...)
				}
			}
		}
	}
	return hits
}

// gameMarkers are entries that show a folder is a real Minecraft install or
// launcher data dir, as opposed to e.g. a Gradle cache or a project that just
// has "minecraft" in its name.
var gameMarkers = []string{
	"mods", "instances", "clusters", "profiles", ".minecraft", "minecraft",
	"launcher_profiles.json", "options.txt", "modpacks",
}

func looksLikeGameDir(dir string) bool {
	for _, m := range gameMarkers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}

// launcherDirs validates a name-matched sweep hit. If the folder itself looks
// like a game/launcher dir it is returned as-is. Otherwise it may just be a
// container (e.g. D:\Minecraft holding OneClient and PrismLauncher), so its
// immediate children that look like game dirs are returned instead.
func launcherDirs(dir string) []string {
	if looksLikeGameDir(dir) {
		return []string{dir}
	}
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		child := filepath.Join(dir, e.Name())
		if st, err := os.Stat(child); err == nil && st.IsDir() && looksLikeGameDir(child) {
			out = append(out, child)
		}
	}
	return out
}

func isLauncherName(name string) bool {
	n := strings.ToLower(name)
	for _, k := range launcherKeywords {
		if strings.Contains(n, k.key) {
			return true
		}
	}
	return false
}

// skipDirs are directory names we never descend into: either irrelevant/huge,
// or known to be large reparse points in relocated launcher setups.
var skipDirs = map[string]bool{
	"node_modules": true, "assets": true, "libraries": true, "versions": true,
	"resourcepacks": true, "shaderpacks": true, ".git": true, "cache": true,
	"natives": true, "meta": true, "icons": true, "iconthemes": true,
	"translations": true, "themes": true, "logs-archive": true,
	// bulky game data that never holds the mods folder
	"metadata": true, "saves": true, "worlds": true, "backups": true,
	"screenshots": true, "crash-reports": true, ".cache": true,
	"config": true, "datapacks": true, "flashback": true, "replay_recordings": true,
}

// findModsDirs walks root up to maxDepth levels deep and returns every
// directory named "mods" (case-insensitive) that contains at least one jar.
//
// Unlike filepath.WalkDir, this walker FOLLOWS directory junctions and symlinks
// (deciding dir-ness with os.Stat, which resolves reparse points). That matters
// because relocated launchers — e.g. %APPDATA%\PrismLauncher junctioned to
// another drive — are invisible to WalkDir. A visited set keyed on the resolved
// path prevents infinite loops when a junction points back up the tree.
func findModsDirs(root string, maxDepth int) []string {
	var found []string
	visited := map[string]bool{}

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			resolved = filepath.Clean(dir)
		}
		key := strings.ToLower(resolved)
		if visited[key] {
			return
		}
		visited[key] = true

		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			full := filepath.Join(dir, name)
			// os.Stat (not e.IsDir) so junctions/symlinks resolve to their target.
			info, err := os.Stat(full)
			if err != nil || !info.IsDir() {
				continue
			}
			lb := strings.ToLower(name)
			if lb == "mods" && dirHasJar(full) {
				found = append(found, full)
				continue // no need to descend into a mods folder
			}
			if skipDirs[lb] {
				continue
			}
			walk(full, depth+1)
		}
	}
	walk(root, 0)
	return found
}

func dirHasJar(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := strings.ToLower(e.Name())
		if strings.HasSuffix(n, ".jar") || strings.HasSuffix(n, ".jar.disabled") {
			return true
		}
	}
	return false
}

// join returns "" if base is empty, otherwise filepath.Join(base, parts...).
func join(base string, parts ...string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, parts...)...)
}
