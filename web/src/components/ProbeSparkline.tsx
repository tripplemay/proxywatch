import { useEffect, useState } from 'react'

type Probe = { ts_ms: number; ok: boolean }

export function ProbeSparkline({ apiKey }: { apiKey: string }) {
  const [data, setData] = useState<Probe[]>([])
  useEffect(() => {
    const load = () =>
      fetch('/api/probes?limit=60&kind=active', { headers: { Authorization: `Bearer ${apiKey}` } })
        .then((r) => r.json())
        .then((d: Probe[]) => setData(d.slice().reverse()))
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [apiKey])

  const W = 600
  const H = 60
  const n = data.length
  return (
    <section>
      <h2>Active probes (last {n})</h2>
      <svg width="100%" viewBox={`0 0 ${W} ${H}`} style={{ background: '#111827', borderRadius: 4 }}>
        {data.map((p, i) => (
          <rect
            key={i}
            x={n > 0 ? (i / n) * W : 0}
            y={p.ok ? H * 0.4 : H * 0.1}
            width={n > 0 ? W / n - 1 : 0}
            height={p.ok ? H * 0.5 : H * 0.8}
            fill={p.ok ? '#10b981' : '#ef4444'}
          />
        ))}
      </svg>
    </section>
  )
}
