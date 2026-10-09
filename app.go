package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"exo/engine"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is bound to the frontend: its exported methods are callable from JS.
type App struct {
	ctx context.Context

	mu         sync.Mutex
	running    bool
	reportPath string
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) domReady(ctx context.Context) { roundCorners() }

// --- data sent to the frontend ---

// ModDTO is one mod as the UI shows it.
type ModDTO struct {
	File     string   `json:"file"`
	Launcher string   `json:"launcher"`
	Instance string   `json:"instance"`
	Verdict  string   `json:"verdict"` // inspected | verified | unknown | warn | flagged
	Reasons  []string `json:"reasons"`
}

type InstanceDTO struct {
	Launcher string `json:"launcher"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Mods     int    `json:"mods"`
	OK       int    `json:"ok"`
	Unknown  int    `json:"unknown"`
	Warn     int    `json:"warn"`
	Flagged  int    `json:"flagged"`
}

type LogDTO struct {
	Client  string `json:"client"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Excerpt string `json:"excerpt"`
}

// ProgressDTO is a live snapshot, sent about ten times a second.
type ProgressDTO struct {
	Stage     string        `json:"stage"` // discover | inspect | verify | logs
	Done      int           `json:"done"`
	Total     int           `json:"total"`
	Rate      float64       `json:"rate"`    // jars per second
	Elapsed   float64       `json:"elapsed"` // seconds since the scan started
	Workers   []string      `json:"workers"` // current jar per worker ("" = idle)
	Flagged   int           `json:"flagged"`
	Warn      int           `json:"warn"`
	Recent    []ModDTO      `json:"recent"` // newest first
	Instances []InstanceDTO `json:"instances"`
}

// ResultDTO is the finished scan.
type ResultDTO struct {
	Overall    string        `json:"overall"` // clean | inconclusive | suspicious | empty
	Mods       int           `json:"mods"`
	Verified   int           `json:"verified"`
	Unknown    int           `json:"unknown"`
	Warn       int           `json:"warn"`
	Flagged    int           `json:"flagged"`
	LogHits    int           `json:"logHits"`
	Online     bool          `json:"online"`
	Duration   float64       `json:"duration"`
	ReportPath string        `json:"reportPath"`
	ReportErr  string        `json:"reportErr"`
	Instances  []InstanceDTO `json:"instances"`
	Findings   []ModDTO      `json:"findings"` // flagged, then warnings
	Unknowns   []ModDTO      `json:"unknowns"`
	Logs       []LogDTO      `json:"logs"`
}

func verdictName(v engine.Verdict) string {
	switch v {
	case engine.VerdictVerified:
		return "verified"
	case engine.VerdictWarn:
		return "warn"
	case engine.VerdictFlagged:
		return "flagged"
	}
	return "unknown"
}

func toModDTO(m engine.ModResult, verdict string) ModDTO {
	reasons := m.Reasons
	if reasons == nil {
		reasons = []string{} // send [] rather than null to the UI
	}
	return ModDTO{File: m.FileName, Launcher: m.Instance.Launcher, Instance: m.Instance.Name, Verdict: verdict, Reasons: reasons}
}

func instanceDTOs(instances []engine.Instance, mods []engine.ModResult) []InstanceDTO {
	out := make([]InstanceDTO, len(instances))
	index := map[string]int{}
	for i, inst := range instances {
		out[i] = InstanceDTO{Launcher: inst.Launcher, Name: inst.Name, Path: inst.ModsDir}
		index[inst.ModsDir] = i
	}
	for _, m := range mods {
		i, ok := index[m.Instance.ModsDir]
		if !ok {
			continue
		}
		out[i].Mods++
		switch m.Verdict {
		case engine.VerdictVerified:
			out[i].OK++
		case engine.VerdictUnknown:
			out[i].Unknown++
		case engine.VerdictWarn:
			out[i].Warn++
		case engine.VerdictFlagged:
			out[i].Flagged++
		}
	}
	return out
}

// --- scan ---

// live is the shared state the worker pool updates and the ticker reads.
type live struct {
	mu        sync.Mutex
	stage     string
	start     time.Time
	inspectAt time.Time
	done      int
	total     int
	workers   []string
	flagged   int
	warn      int
	recent    []ModDTO
	instances []InstanceDTO
}

const recentMax = 40

func (l *live) snapshot() ProgressDTO {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := ProgressDTO{
		Stage: l.stage, Done: l.done, Total: l.total,
		Elapsed: time.Since(l.start).Seconds(),
		Flagged: l.flagged, Warn: l.warn,
		Workers:   append([]string{}, l.workers...),
		Recent:    append([]ModDTO{}, l.recent...),
		Instances: l.instances,
	}
	if !l.inspectAt.IsZero() {
		if s := time.Since(l.inspectAt).Seconds(); s > 0 {
			p.Rate = float64(l.done) / s
		}
	}
	return p
}

func (l *live) setStage(s string) {
	l.mu.Lock()
	l.stage = s
	l.mu.Unlock()
}

// StartScan runs a full scan in the background. Progress arrives as
// "scan:progress" events and the result as "scan:done". It returns false if a
// scan is already running.
func (a *App) StartScan() bool {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return false
	}
	a.running = true
	a.mu.Unlock()

	go a.scan()
	return true
}

func (a *App) scan() {
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()

	st := &live{stage: "discover", start: time.Now(), workers: make([]string, engine.Workers)}
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				runtime.EventsEmit(a.ctx, "scan:progress", st.snapshot())
			}
		}
	}()

	instances := engine.FindInstances()
	refs := engine.CollectJars(instances)
	st.mu.Lock()
	st.instances = instanceDTOs(instances, nil)
	st.stage, st.total, st.inspectAt = "inspect", len(refs), time.Now()
	st.mu.Unlock()

	mods := engine.InspectAll(refs, engine.ScanHooks{
		Start: func(w int, name string) {
			st.mu.Lock()
			st.workers[w] = name
			st.mu.Unlock()
		},
		Done: func(w int, r engine.ModResult) {
			st.mu.Lock()
			st.done++
			st.workers[w] = ""
			verdict := "inspected"
			switch r.Verdict {
			case engine.VerdictFlagged:
				st.flagged++
				verdict = "flagged"
			case engine.VerdictWarn:
				st.warn++
				verdict = "warn"
			}
			st.recent = append([]ModDTO{toModDTO(r, verdict)}, st.recent...)
			if len(st.recent) > recentMax {
				st.recent = st.recent[:recentMax]
			}
			st.mu.Unlock()
		},
	})

	st.setStage("verify")
	online := true
	if len(mods) > 0 {
		online = engine.VerifyModrinth(mods)
	}
	st.setStage("logs")
	logFlags := engine.ScanLogs(instances)

	sum := engine.NewSummary(instances, mods, logFlags, online, time.Since(st.start))
	res := ResultDTO{
		Overall: string(sum.Overall()), Mods: len(mods),
		Verified: sum.Verified, Unknown: sum.Unknown, Warn: sum.Warn, Flagged: sum.Flagged,
		LogHits: len(logFlags), Online: online, Duration: sum.Duration.Seconds(),
		Instances: instanceDTOs(instances, mods),
		Findings:  []ModDTO{},
		Unknowns:  []ModDTO{},
		Logs:      []LogDTO{},
	}
	for _, want := range []engine.Verdict{engine.VerdictFlagged, engine.VerdictWarn} {
		for _, m := range mods {
			if m.Verdict == want {
				res.Findings = append(res.Findings, toModDTO(m, verdictName(m.Verdict)))
			}
		}
	}
	for _, m := range mods {
		if m.Verdict == engine.VerdictUnknown {
			res.Unknowns = append(res.Unknowns, toModDTO(m, "unknown"))
		}
	}
	sort.Slice(res.Unknowns, func(i, j int) bool { return res.Unknowns[i].File < res.Unknowns[j].File })
	for _, lf := range logFlags {
		res.Logs = append(res.Logs, LogDTO{Client: lf.Client, File: lf.File, Line: lf.Line, Excerpt: lf.Excerpt})
	}

	res.ReportPath = reportPath()
	if err := engine.WriteReport(res.ReportPath, sum); err != nil {
		res.ReportErr = err.Error()
	}
	a.mu.Lock()
	a.reportPath = res.ReportPath
	a.mu.Unlock()

	close(stop)
	runtime.EventsEmit(a.ctx, "scan:progress", st.snapshot())
	runtime.EventsEmit(a.ctx, "scan:done", res)
	debug.FreeOSMemory() // the widget idles after a scan; give the memory back
}

// reportPath puts result.txt next to the exe, like the CLI does when run
// from its own folder.
func reportPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "result.txt")
	}
	return "result.txt"
}

// OpenReport opens result.txt in the default text editor.
func (a *App) OpenReport() {
	a.mu.Lock()
	p := a.reportPath
	a.mu.Unlock()
	if p != "" {
		exec.Command("explorer.exe", p).Start()
	}
}

// ShowReportInFolder opens Explorer with result.txt selected.
func (a *App) ShowReportInFolder() {
	a.mu.Lock()
	p := a.reportPath
	a.mu.Unlock()
	if p != "" {
		exec.Command("explorer.exe", "/select,", p).Start()
	}
}

// Version returns the app version for the footer.
func (a *App) Version() string { return engine.Version }

// SetExpanded grows or shrinks the widget for the details panel. If the
// taller window would run off the bottom of the screen, it moves up to fit.
func (a *App) SetExpanded(expanded bool) {
	h := collapsedHeight
	if expanded {
		h = expandedHeight
	}
	runtime.WindowSetSize(a.ctx, widgetWidth, h)
	defer roundCorners() // the rounded clip region must match the new size

	screens, err := runtime.ScreenGetAll(a.ctx)
	if err != nil {
		return
	}
	for _, s := range screens {
		if !s.IsCurrent {
			continue
		}
		x, y := runtime.WindowGetPosition(a.ctx)
		const margin = 12
		const taskbar = 48 // keep clear of a bottom taskbar
		if max := s.Size.Height - h - margin - taskbar; y > max {
			y = max
		}
		if y < margin {
			y = margin
		}
		runtime.WindowSetPosition(a.ctx, x, y)
	}
}
