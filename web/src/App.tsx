import { useEffect, useState } from 'react'
import { fetchStatus, getKey, setKey, Status } from './api'
import { Settings } from './components/Settings'
import { ProbeSparkline } from './components/ProbeSparkline'
import { IncidentTable } from './components/IncidentTable'
import { RotationTable } from './components/RotationTable'
import './styles.css'

export default function App() {
  const [keyInput, setKeyInput] = useState(getKey())
  const [status, setStatus] = useState<Status | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!getKey()) return
    const refresh = async () => {
      try {
        setStatus(await fetchStatus())
        setError(null)
      } catch (e) {
        setError(String(e))
      }
    }
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [keyInput])

  if (!getKey()) {
    return (
      <div className="login">
        <h1>proxywatch</h1>
        <p>Enter your PROXYWATCH_KEY to continue.</p>
        <input value={keyInput} onChange={(e) => setKeyInput(e.target.value)} type="password" />
        <button onClick={() => { setKey(keyInput); setKeyInput(keyInput) }}>Save</button>
      </div>
    )
  }

  return (
    <div className="app">
      <header>
        <h1>proxywatch</h1>
        <span className="version">v{status?.version || '?'}</span>
      </header>
      {error && <div className="error">{error}</div>}
      <section className="status-card">
        <h2>State</h2>
        <div className={`state state-${status?.state || 'unknown'}`}>{status?.state || '...'}</div>
        <div className="exit-ip">Exit IP: {status?.exit_ip || '(unknown)'}</div>
      </section>
      <section className="probe-card">
        <h2>Last active probe</h2>
        {status?.last_active_probe ? (
          <ul>
            <li>HTTP: {status.last_active_probe.http_code} ({status.last_active_probe.ok ? 'OK' : 'FAIL'})</li>
            <li>Latency: {status.last_active_probe.latency_ms} ms</li>
            <li>Time: {new Date(status.last_active_probe.ts_ms).toLocaleTimeString()}</li>
          </ul>
        ) : <p>No probes yet</p>}
      </section>

      {(status?.state === 'ROTATING' || status?.state === 'SUSPECT') && (
        <button className="confirm-btn" onClick={async () => {
          await fetch('/api/confirm-rotation', { method: 'POST', headers: { Authorization: `Bearer ${getKey()}` } })
        }}>I rotated, re-verify</button>
      )}

      {status?.state === 'ALERT_ONLY' && (
        <button className="resume-btn" onClick={async () => {
          await fetch('/api/resume-automation', { method: 'POST', headers: { Authorization: `Bearer ${getKey()}` } })
        }}>Resume automation</button>
      )}

      <ProbeSparkline apiKey={getKey()} />
      <IncidentTable apiKey={getKey()} />
      <RotationTable apiKey={getKey()} />

      <Settings apiKey={getKey()} />

      <footer>
        <button onClick={() => { localStorage.removeItem('proxywatch.key'); location.reload() }}>Logout</button>
      </footer>
    </div>
  )
}
