package main

import (
	"os"
	"path/filepath"
	"strings"
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

// findInstances locates Minecraft mod folders across every launcher we know of.
// It deep-scans each launcher's root directory for folders literally named
// "mods" that contain at least one jar, so it is not limited to .minecraft.
// Everything lives under the current user's profile, so no admin is required.
func findInstances() []Instance {
	appdata := os.Getenv("APPDATA")       // Roaming
	local := os.Getenv("LOCALAPPDATA")    // Local
	home, _ := os.UserHomeDir()           // %USERPROFILE%
	docs := filepath.Join(home, "Documents")

	roots := []launcherRoot{
		{"Minecraft (default)", join(appdata, ".minecraft")},
		{"Modrinth App", join(appdata, "ModrinthApp")},
		{"Modrinth (Theseus)", join(appdata, "com.modrinth.theseus")},
		{"Prism Launcher", join(appdata, "PrismLauncher")},
		{"PolyMC", join(appdata, "PolyMC")},
		{"MultiMC", join(appdata, ".multimc")},
		{"MultiMC", join(home, "MultiMC")},
		{"MultiMC", join(docs, "MultiMC")},
		{"CurseForge", join(home, "curseforge")},
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
		{"Lunar Client", join(home, ".lunarclient")},
		{"Feather Client", join(appdata, ".feather")},
		{"Badlion Client", join(appdata, "Badlion Client")},
		{"TLauncher", join(appdata, ".tlauncher")},
		{"SKLauncher", join(appdata, ".minecraft_sklauncher")},
		{"Legacy Launcher", join(appdata, ".tlauncherlegacy")},
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
			if name == "." || name == string(filepath.Separator) {
				name = "main"
			}
			out = append(out, Instance{
				Launcher: r.Label,
				Name:     name,
				ModsDir:  modsDir,
				LogDirs:  []string{filepath.Join(parent, "logs")},
			})
		}
	}
	return out
}

// skipDirs are directory names we never descend into: either irrelevant/huge,
// or known to be large reparse points in relocated launcher setups.
var skipDirs = map[string]bool{
	"node_modules": true, "assets": true, "libraries": true, "versions": true,
	"resourcepacks": true, "shaderpacks": true, ".git": true, "cache": true,
	"natives": true, "meta": true, "icons": true, "iconthemes": true,
	"translations": true, "themes": true, "logs-archive": true,
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
