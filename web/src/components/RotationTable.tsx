import { useEffect, useState } from 'react'

type Rotation = {
  ID: number
  StartedAt: string
  EndedAt: string | null
  OldIP: string
  NewIP: string
  DetectionMethod: string
  OK: boolean
}

export function RotationTable({ apiKey }: { apiKey: string }) {
  const [rows, setRows] = useState<Rotation[]>([])
  useEffect(() => {
    fetch('/api/rotations', { headers: { Authorization: `Bearer ${apiKey}` } })
      .then((r) => r.json())
      .then((d) => setRows(d || []))
  }, [apiKey])

  return (
    <section>
      <h2>Rotations</h2>
      {rows.length === 0 ? <p>No rotations recorded</p> : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Started</th><th>Old IP</th><th>New IP</th><th>Method</th><th>OK</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.ID}>
                <td>{new Date(r.StartedAt).toLocaleString()}</td>
                <td>{r.OldIP || '—'}</td>
                <td>{r.NewIP || '—'}</td>
                <td>{r.DetectionMethod}</td>
                <td>{r.OK ? '✓' : '✗'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  )
}
