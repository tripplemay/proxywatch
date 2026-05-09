const KEY_STORAGE = 'proxywatch.key'

export type Status = {
  version: string
  state: string
  exit_ip?: string
  last_active_probe?: {
    ts_ms: number
    http_code: number
    latency_ms: number
    exit_ip?: string
    ok: boolean
    error?: string
  }
}

export const getKey = () => localStorage.getItem(KEY_STORAGE) || ''
export const setKey = (k: string) => localStorage.setItem(KEY_STORAGE, k)

async function authedFetch(path: string, init: RequestInit = {}) {
  const r = await fetch(path, {
    ...init,
    headers: { ...(init.headers || {}), Authorization: `Bearer ${getKey()}` },
  })
  if (!r.ok) throw new Error(`${r.status} ${r.statusText}`)
  return r.json()
}

export const fetchStatus = (): Promise<Status> => authedFetch('/api/status')
