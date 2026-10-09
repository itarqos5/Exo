// Ring is the hero progress indicator.
//   mode "busy":     a spinning arc (stages without a measurable count)
//   mode "progress": a gradient arc filled to `ratio`
//   mode "done":     a full ring in the verdict's tone
const SIZE = 184
const STROKE = 10
const R = (SIZE - STROKE) / 2 - 6
const C = 2 * Math.PI * R
const MID = SIZE / 2

export function Ring({ ratio, mode, tone, children }) {
  const offset = C * (1 - Math.max(0, Math.min(1, ratio)))
  return (
    <div className={`ring ring-${mode} ${tone ? `tone-${tone}` : ''}`} style={{ width: SIZE, height: SIZE }}>
      <svg width={SIZE} height={SIZE} viewBox={`0 0 ${SIZE} ${SIZE}`}>
        {/* faint tick marks around the ring */}
        <g className="ring-ticks">
          {Array.from({ length: 60 }, (_, i) => {
            const a = (i / 60) * 2 * Math.PI
            const r1 = R + STROKE / 2 + 5
            const r2 = r1 + (i % 5 === 0 ? 5 : 3)
            return (
              <line
                key={i}
                x1={MID + r1 * Math.cos(a)}
                y1={MID + r1 * Math.sin(a)}
                x2={MID + r2 * Math.cos(a)}
                y2={MID + r2 * Math.sin(a)}
              />
            )
          })}
        </g>
        <circle className="ring-track" cx={MID} cy={MID} r={R} strokeWidth={STROKE} fill="none" />
      </svg>
      {/* The arc lives in its own layer so "busy" mode can spin the whole
          layer around its centre. */}
      <svg className="ring-arc-layer" width={SIZE} height={SIZE} viewBox={`0 0 ${SIZE} ${SIZE}`}>
        <defs>
          <linearGradient id="ringGrad" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stopColor="#ddd6fe" />
            <stop offset="55%" stopColor="#a78bfa" />
            <stop offset="100%" stopColor="#7c3aed" />
          </linearGradient>
        </defs>
        <circle
          className="ring-arc"
          cx={MID}
          cy={MID}
          r={R}
          strokeWidth={STROKE}
          fill="none"
          strokeLinecap="round"
          strokeDasharray={mode === 'busy' ? `${C * 0.22} ${C}` : C}
          strokeDashoffset={mode === 'progress' ? offset : 0}
          transform={`rotate(-90 ${MID} ${MID})`}
        />
      </svg>
      <div className="ring-center">{children}</div>
    </div>
  )
}
