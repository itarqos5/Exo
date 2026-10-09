import { useEffect, useRef, useState } from 'react'
import { StartScan, OpenReport, ShowReportInFolder, SetExpanded, Version } from '../wailsjs/go/main/App'
import { EventsOn, WindowMinimise, Quit } from '../wailsjs/runtime/runtime'
import { Ring } from './Ring'
import { Details } from './Details'
import { Icon } from './icons'

const STAGES = [
  { key: 'discover', label: 'Discover' },
  { key: 'inspect', label: 'Inspect' },
  { key: 'verify', label: 'Verify' },
  { key: 'logs', label: 'Logs' },
]

const VERDICTS = {
  clean: { tone: 'green', icon: 'shieldCheck', title: 'Looks clean', sub: () => 'Every mod verified on Modrinth' },
  inconclusive: {
    tone: 'yellow',
    icon: 'shieldQuestion',
    title: 'Inconclusive',
    sub: (r) =>
      r.warn > 0
        ? `${r.warn} obfuscated mod${r.warn === 1 ? '' : 's'} need a manual look`
        : `No known cheats · ${r.unknown} mod${r.unknown === 1 ? '' : 's'} not on Modrinth`,
  },
  suspicious: {
    tone: 'red',
    icon: 'shieldAlert',
    title: 'Suspicious',
    sub: (r) => `${r.flagged + r.logHits} known cheat signature${r.flagged + r.logHits === 1 ? '' : 's'} matched`,
  },
  empty: { tone: 'muted', icon: 'folder', title: 'No mods found', sub: () => 'No Minecraft mod folders on this PC' },
}

// Overall progress across all stages, 0..1.
function overallRatio(p) {
  if (!p) return 0
  switch (p.stage) {
    case 'discover':
      return 0.03
    case 'inspect':
      return 0.05 + 0.8 * (p.total ? p.done / p.total : 1)
    case 'verify':
      return 0.9
    case 'logs':
      return 0.97
  }
  return 0
}

function statusText(p) {
  switch (p?.stage) {
    case 'inspect':
      return ['Scanning mods', `${p.done} of ${p.total} · ${Math.round(p.rate)} jars/s`]
    case 'verify':
      return ['Verifying hashes', `Checking ${p.total} files against Modrinth`]
    case 'logs':
      return ['Reading logs', 'Looking for cheat-client markers']
  }
  return ['Finding launchers', 'Checking 30+ launchers on every drive']
}

// useCountUp animates a number toward its target.
function useCountUp(target) {
  const [value, setValue] = useState(target ?? 0)
  const from = useRef(value)
  useEffect(() => {
    if (target == null) return
    const start = performance.now()
    const a = from.current
    let raf
    const tick = (now) => {
      const t = Math.min(1, (now - start) / 450)
      const v = Math.round(a + (target - a) * (1 - Math.pow(1 - t, 3)))
      setValue(v)
      from.current = v
      if (t < 1) raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [target])
  return value
}

function Stat({ label, value, tone }) {
  const shown = useCountUp(value)
  return (
    <div className={`stat tone-${tone}`}>
      <span className="stat-value">{value == null ? '—' : shown}</span>
      <span className="stat-label">{label}</span>
    </div>
  )
}

function Stepper({ stage, done }) {
  const current = done ? STAGES.length : STAGES.findIndex((s) => s.key === stage)
  return (
    <div className="stepper">
      {STAGES.map((s, i) => {
        const state = i < current ? 'done' : i === current ? 'active' : 'pending'
        return (
          <div key={s.key} className={`step ${state}`}>
            <div className="step-dot">{state === 'done' ? <Icon name="check" size={12} /> : <span />}</div>
            <span className="step-label">{s.label}</span>
          </div>
        )
      })}
    </div>
  )
}

export default function App() {
  const [progress, setProgress] = useState(null)
  const [result, setResult] = useState(null)
  const [expanded, setExpanded] = useState(false)
  const [version, setVersion] = useState('')

  useEffect(() => {
    const offProgress = EventsOn('scan:progress', setProgress)
    const offDone = EventsOn('scan:done', setResult)
    Version().then(setVersion)
    StartScan()
    return () => {
      offProgress()
      offDone()
    }
  }, [])

  const running = !result
  const rescan = () => {
    setResult(null)
    setProgress(null)
    StartScan()
  }
  const toggleDetails = () => {
    const next = !expanded
    setExpanded(next)
    SetExpanded(next)
  }

  const verdict = result ? VERDICTS[result.overall] : null
  const [title, sub] = result ? [verdict.title, verdict.sub(result)] : statusText(progress)
  const pct = Math.round(overallRatio(progress) * 100)

  return (
    <div className={`app ${result ? `verdict-${verdict.tone}` : ''}`}>
      <header className="titlebar">
        <div className="brand">
          <div className="logo">
            <Icon name="logo" size={14} />
          </div>
          <span className="brand-name">Exo</span>
          <span className="brand-sub">Integrity Scanner</span>
        </div>
        <div className="window-buttons">
          <button className="win-btn" onClick={WindowMinimise} title="Minimize">
            <Icon name="minimize" size={14} />
          </button>
          <button className="win-btn close" onClick={Quit} title="Close">
            <Icon name="close" size={14} />
          </button>
        </div>
      </header>

      <section className="hero">
        <Ring
          ratio={overallRatio(progress)}
          mode={result ? 'done' : progress?.stage === 'inspect' ? 'progress' : 'busy'}
          tone={verdict?.tone}
        >
          {result ? (
            <div className={`ring-verdict tone-${verdict.tone}`}>
              <Icon name={verdict.icon} size={46} />
            </div>
          ) : (
            <div className="ring-pct">
              <span className="ring-num">{pct}</span>
              <span className="ring-unit">%</span>
            </div>
          )}
        </Ring>
        <h1 className={`status-title ${result ? `tone-${verdict.tone}` : ''}`}>{title}</h1>
        <p className="status-sub">{sub}</p>
        {result && (
          <p className="status-meta">
            {result.mods} mods · {result.instances.length} instance{result.instances.length === 1 ? '' : 's'} ·{' '}
            {result.duration.toFixed(1)}s{!result.online && ' · Modrinth offline'}
          </p>
        )}
      </section>

      <Stepper stage={progress?.stage} done={!!result} />

      <div className="stats">
        <Stat label="Verified" value={result ? result.verified : null} tone="green" />
        <Stat label="Unknown" value={result ? result.unknown : null} tone="yellow" />
        <Stat label="Warnings" value={result ? result.warn : progress?.warn ?? 0} tone="orange" />
        <Stat label="Flagged" value={result ? result.flagged + result.logHits : progress?.flagged ?? 0} tone="red" />
      </div>

      <button className={`details-toggle ${expanded ? 'open' : ''}`} onClick={toggleDetails}>
        <span>{expanded ? 'Hide additional details' : 'Show additional details'}</span>
        <Icon name="chevron" size={14} />
      </button>

      {expanded && <Details progress={progress} result={result} />}

      <footer className="footer">
        <div className="footer-buttons">
          <button className="btn primary" disabled={running} onClick={OpenReport}>
            <Icon name="file" size={15} /> Open report
          </button>
          <button className="btn ghost" disabled={running} onClick={rescan}>
            <Icon name="refresh" size={15} /> Scan again
          </button>
        </div>
        <div className="footer-note">
          <span>
            v{version} · detects known clients · one signal, not proof
          </span>
          {result && !result.reportErr && (
            <button className="link" onClick={ShowReportInFolder}>
              Show in folder
            </button>
          )}
        </div>
      </footer>
    </div>
  )
}
