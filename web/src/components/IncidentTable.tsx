import { useEffect, useState } from 'react'

type Incident = {
  ID: number
  StartedAt: string
  EndedAt: string | null
  TriggerReason: string
  TerminalState: string
  RotationCount: number
}

export function IncidentTable({ apiKey }: { apiKey: string }) {
  const [rows, setRows] = useState<Incident[]>([])
  useEffect(() => {
    fetch('/api/incidents', { headers: { Authorization: `Bearer ${apiKey}` } })
      .then((r) => r.json())
      .then((d) => setRows(d || []))
  }, [apiKey])

  return (
    <section>
      <h2>Incidents</h2>
      {rows.length === 0 ? <p>No incidents recorded</p> : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Started</th><th>Ended</th><th>Trigger</th><th>Final state</th><th>Rotations</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.ID}>
                <td>{new Date(r.StartedAt).toLocaleString()}</td>
                <td>{r.EndedAt ? new Date(r.EndedAt).toLocaleString() : '— open —'}</td>
                <td>{r.TriggerReason}</td>
                <td>{r.TerminalState || '—'}</td>
                <td>{r.RotationCount}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  )
}
