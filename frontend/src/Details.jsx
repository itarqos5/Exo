import { useState } from 'react'
import { Icon } from './icons'

function Section({ icon, title, count, children, tone }) {
  return (
    <section className={`card ${tone ? `tone-${tone}` : ''}`}>
      <header className="card-head">
        <Icon name={icon} size={14} />
        <span>{title}</span>
        {count != null && <span className="card-count">{count}</span>}
      </header>
      {children}
    </section>
  )
}

function Workers({ workers }) {
  return (
    <Section icon="cpu" title="Workers" count={`${workers.filter(Boolean).length}/${workers.length} busy`}>
      <ul className="workers">
        {workers.map((w, i) => (
          <li key={i} className={w ? 'busy' : 'idle'}>
            <span className="worker-id">{i + 1}</span>
            {w ? <span className="spinner" /> : <span className="idle-dot" />}
            <span className="mono ellipsis">{w ? w.replace(/\.jar$/, '') : 'idle'}</span>
          </li>
        ))}
      </ul>
    </Section>
  )
}

const FEED_LABEL = { inspected: 'checked', flagged: 'flagged', warn: 'warning' }

function Feed({ recent }) {
  return (
    <Section icon="list" title="Recently scanned" count={recent.length ? `last ${recent.length}` : null}>
      {recent.length === 0 ? (
        <p className="empty">Waiting for the first jar…</p>
      ) : (
        <ul className="feed">
          {recent.map((m, i) => (
            <li key={`${m.file}-${m.instance}-${i}`} className={`feed-row v-${m.verdict}`}>
              <span className="dot" />
              <span className="mono ellipsis">{m.file.replace(/\.jar$/, '')}</span>
              <span className="feed-meta">{FEED_LABEL[m.verdict] ?? m.verdict}</span>
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

function Instances({ instances, final }) {
  if (!instances?.length) return null
  return (
    <Section icon="layers" title="Instances" count={instances.length}>
      <ul className="instances">
        {instances.map((inst) => {
          const total = inst.mods || 1
          return (
            <li key={inst.path} title={inst.path}>
              <div className="inst-top">
                <span className="inst-badge">{inst.launcher.slice(0, 1)}</span>
                <div className="inst-text">
                  <span className="ellipsis">{inst.name}</span>
                  <span className="inst-launcher ellipsis">{inst.launcher}</span>
                </div>
                {final && <span className="inst-mods">{inst.mods} mods</span>}
              </div>
              {final && inst.mods > 0 && (
                <div className="minibar">
                  <span className="seg green" style={{ width: `${(inst.ok / total) * 100}%` }} />
                  <span className="seg yellow" style={{ width: `${(inst.unknown / total) * 100}%` }} />
                  <span className="seg orange" style={{ width: `${(inst.warn / total) * 100}%` }} />
                  <span className="seg red" style={{ width: `${(inst.flagged / total) * 100}%` }} />
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </Section>
  )
}

function Findings({ result }) {
  const findings = result.findings ?? []
  const logs = result.logs ?? []
  if (findings.length === 0 && logs.length === 0) {
    return (
      <div className="all-clear">
        <Icon name="check" size={15} />
        No known cheat signatures, obfuscation or log markers found.
      </div>
    )
  }
  return (
    <Section icon="alert" title="Findings" count={findings.length + logs.length} tone="red">
      <ul className="findings">
        {findings.map((m, i) => (
          <li key={i} className={`finding v-${m.verdict}`}>
            <div className="finding-top">
              <span className={`pill v-${m.verdict}`}>{m.verdict === 'flagged' ? 'FLAGGED' : 'WARNING'}</span>
              <span className="mono ellipsis">{m.file}</span>
            </div>
            <span className="finding-where">
              {m.launcher} · {m.instance}
            </span>
            <ul className="reasons">
              {(m.reasons ?? []).map((r, j) => (
                <li key={j}>{r}</li>
              ))}
            </ul>
          </li>
        ))}
        {logs.map((l, i) => (
          <li key={`log-${i}`} className="finding v-flagged">
            <div className="finding-top">
              <span className="pill v-flagged">LOG</span>
              <span className="ellipsis">{l.client}</span>
            </div>
            <span className="finding-where ellipsis" title={`${l.file}:${l.line}`}>
              {l.file}:{l.line}
            </span>
            <ul className="reasons">
              <li className="mono">{l.excerpt}</li>
            </ul>
          </li>
        ))}
      </ul>
    </Section>
  )
}

function Unknowns({ unknowns }) {
  const [open, setOpen] = useState(false)
  if (!unknowns?.length) return null
  // Group identical jars that sit in several instances.
  const groups = []
  const index = new Map()
  for (const m of unknowns) {
    const name = m.file.replace(/\.jar$/, '')
    if (index.has(name)) groups[index.get(name)].count++
    else {
      index.set(name, groups.length)
      groups.push({ name, count: 1 })
    }
  }
  return (
    <section className="card tone-yellow">
      <button className="card-head clickable" onClick={() => setOpen(!open)}>
        <Icon name="help" size={14} />
        <span>Not on Modrinth</span>
        <span className="card-count">{unknowns.length}</span>
        <span className={`chev ${open ? 'open' : ''}`}>
          <Icon name="chevron" size={13} />
        </span>
      </button>
      <p className="hint">CurseForge-only or private builds. Not proof of anything by itself.</p>
      {open && (
        <ul className="chips">
          {groups.map((g) => (
            <li key={g.name} className="chip mono" title={g.name}>
              <span className="ellipsis">{g.name}</span>
              {g.count > 1 && <span className="chip-count">×{g.count}</span>}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

export function Details({ progress, result }) {
  const inspecting = !result && progress?.stage === 'inspect'
  return (
    <div className="details">
      {result && <Findings result={result} />}
      {result && <Unknowns unknowns={result.unknowns} />}
      {inspecting && <Workers workers={progress.workers ?? []} />}
      <Feed recent={progress?.recent ?? []} />
      <Instances instances={result ? result.instances : progress?.instances} final={!!result} />
    </div>
  )
}
