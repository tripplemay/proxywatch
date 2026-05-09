import { useEffect, useState } from 'react'

export function Settings({ apiKey }: { apiKey: string }) {
  const [vals, setVals] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const [savedAt, setSavedAt] = useState<string | null>(null)
  const fields = [
    'active_probe_interval_seconds',
    'passive_threshold',
    'active_failure_threshold',
    'suspect_observation_seconds',
    'cooldown_seconds',
    'telegram_bot_token',
    'telegram_chat_id',
  ]

  useEffect(() => {
    fetch('/api/settings', { headers: { Authorization: `Bearer ${apiKey}` } })
      .then((r) => r.json())
      .then(setVals)
  }, [apiKey])

  async function save() {
    setSaving(true)
    try {
      await fetch('/api/settings', {
        method: 'PUT',
        headers: { Authorization: `Bearer ${apiKey}`, 'Content-Type': 'application/json' },
        body: JSON.stringify(vals),
      })
      setSavedAt(new Date().toLocaleTimeString())
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="settings-card">
      <h2>Settings</h2>
      {fields.map((f) => (
        <div key={f} className="settings-row">
          <label>{f}</label>
          <input
            value={vals[f] || ''}
            onChange={(e) => setVals({ ...vals, [f]: e.target.value })}
            type={f.includes('token') ? 'password' : 'text'}
          />
        </div>
      ))}
      <button onClick={save} disabled={saving}>{saving ? 'saving…' : 'Save'}</button>
      {savedAt && <span className="saved-at"> saved at {savedAt}</span>}
    </section>
  )
}
