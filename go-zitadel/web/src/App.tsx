import { useEffect, useState } from 'react'

function decodeJWT(token: string): Record<string, unknown> | null {
  try { return JSON.parse(atob(token.split('.')[1])) } catch { return null }
}

// =============================================================================
// Types
// =============================================================================
type EndpointResult = { status: number | null; body: unknown; loading: boolean }
type Endpoint = { label: string; method: string; path: string; group: string }

const ROLES = ['admin', 'creator', 'viewer'] as const
const RESOURCES = [
  { type: 'video',   id: 'v1' },
  { type: 'video',   id: 'v2' },
  { type: 'channel', id: 'c1' },
]

const ENDPOINTS: Endpoint[] = [
  { group: 'Auth',     label: 'GET  /health',              method: 'GET',    path: '/health' },
  { group: 'Auth',     label: 'GET  /api/me',               method: 'GET',    path: '/api/me' },
  // Videos — any user can read, creator required to write
  { group: 'Videos',   label: 'GET    /api/videos',         method: 'GET',    path: '/api/videos' },
  { group: 'Videos',   label: 'GET    /api/videos/v1',      method: 'GET',    path: '/api/videos/v1' },
  { group: 'Videos',   label: 'POST   /api/videos',         method: 'POST',   path: '/api/videos' },
  { group: 'Videos',   label: 'PUT    /api/videos/v1',      method: 'PUT',    path: '/api/videos/v1' },
  { group: 'Videos',   label: 'DELETE /api/videos/v1',      method: 'DELETE', path: '/api/videos/v1' },
  // Channels — creator-only, only your own channels visible
  { group: 'Channels', label: 'GET    /api/channels',       method: 'GET',    path: '/api/channels' },
  { group: 'Channels', label: 'GET    /api/channels/c1',    method: 'GET',    path: '/api/channels/c1' },
  { group: 'Channels', label: 'POST   /api/channels',       method: 'POST',   path: '/api/channels' },
  { group: 'Channels', label: 'PUT    /api/channels/c1',    method: 'PUT',    path: '/api/channels/c1' },
  { group: 'Channels', label: 'DELETE /api/channels/c1',    method: 'DELETE', path: '/api/channels/c1' },
  // Admin
  { group: 'Admin',    label: 'GET    /api/admin/users',    method: 'GET',    path: '/api/admin/users' },
  { group: 'Admin',    label: 'DELETE /api/admin/users/u1', method: 'DELETE', path: '/api/admin/users/u1' },
  // Debug
  { group: 'Debug',    label: 'GET  /api/debug/perms',      method: 'GET',    path: '/api/debug/perms' },
]

const METHOD_COLOR: Record<string, string> = {
  GET: '#4ade80', POST: '#60a5fa', PUT: '#fbbf24', DELETE: '#f87171',
}

// =============================================================================
// API helpers
// =============================================================================
async function apiFetch(method: string, path: string, token: string | null, body?: unknown) {
  const res = await fetch(path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  let json: unknown
  try { json = await res.json() } catch { json = null }
  return { status: res.status, body: json }
}

// =============================================================================
// Endpoint row
// =============================================================================
function EndpointRow({ ep, token }: { ep: Endpoint; token: string | null }) {
  const [result, setResult] = useState<EndpointResult>({ status: null, body: null, loading: false })

  async function hit() {
    setResult({ status: null, body: null, loading: true })
    try {
      const r = await apiFetch(ep.method, ep.path, token)
      setResult({ ...r, loading: false })
    } catch (e) {
      setResult({ status: null, body: String(e), loading: false })
    }
  }

  const { status, body, loading } = result
  const statusColor = status == null ? '#555' : status < 300 ? '#4ade80' : status < 400 ? '#fbbf24' : '#f87171'

  return (
    <div style={s.row}>
      <div style={s.rowLeft}>
        <span style={{ ...s.badge, color: METHOD_COLOR[ep.method] ?? '#fff' }}>{ep.method}</span>
        <span style={s.path}>{ep.path}</span>
      </div>
      <button style={s.hitBtn} onClick={hit} disabled={loading}>{loading ? '…' : 'Hit'}</button>
      <div style={s.resultBox}>
        {status != null && <span style={{ color: statusColor, fontWeight: 700, marginRight: 6 }}>{status}</span>}
        {body != null && <span style={s.bodyText}>{JSON.stringify(body)}</span>}
      </div>
    </div>
  )
}

// =============================================================================
// Permissions panel
// =============================================================================
function PermissionsPanel({ token }: { token: string | null }) {
  const [perms, setPerms]     = useState<Record<string, string>>({})
  const [pending, setPending] = useState<Record<string, string>>({})
  const [msg, setMsg]         = useState('')

  async function load() {
    if (!token) return
    const r = await apiFetch('GET', '/api/debug/perms', token)
    if (r.status === 200) setPerms(r.body as Record<string, string>)
  }

  useEffect(() => { load() }, [token])

  function currentRole(type: string, id: string) {
    return perms[`${type}/${id}`] ?? 'none'
  }

  function setPendingRole(type: string, id: string, role: string) {
    setPending(p => ({ ...p, [`${type}/${id}`]: role }))
  }

  async function apply(type: string, id: string) {
    const role = pending[`${type}/${id}`]
    if (!role) return
    if (role === 'none') {
      await apiFetch('DELETE', '/api/debug/revoke', token, { resource_type: type, resource_id: id })
    } else {
      await apiFetch('POST', '/api/debug/grant', token, { resource_type: type, resource_id: id, role })
    }
    setMsg(`${type}/${id} → ${role}`)
    await load()
    setTimeout(() => setMsg(''), 2000)
  }

  async function grantAll() {
    for (const { type, id } of RESOURCES) {
      await apiFetch('POST', '/api/debug/grant', token, { resource_type: type, resource_id: id, role: 'creator' })
    }
    await load()
    setMsg('all resources → creator')
    setTimeout(() => setMsg(''), 2000)
  }

  if (!token) return null

  return (
    <div style={s.permPanel}>
      <div style={s.permHeader}>
        <span style={s.groupLabel}>Permissions (permStore)</span>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {msg && <span style={{ color: '#4ade80', fontSize: 12 }}>{msg}</span>}
          <button style={s.grantAllBtn} onClick={grantAll}>Grant all → creator</button>
        </div>
      </div>
      <div style={s.permGrid}>
        {RESOURCES.map(({ type, id }) => {
          const key  = `${type}/${id}`
          const cur  = currentRole(type, id)
          const sel  = pending[key] ?? cur
          return (
            <div key={key} style={s.permRow}>
              <span style={s.permResource}>{key}</span>
              <span style={{ ...s.roleBadge, background: roleColor(cur) }}>{cur}</span>
              <select
                style={s.select}
                value={sel}
                onChange={e => setPendingRole(type, id, e.target.value)}
              >
                <option value="none">none</option>
                {ROLES.map(r => <option key={r} value={r}>{r}</option>)}
              </select>
              <button
                style={{ ...s.hitBtn, opacity: sel !== cur ? 1 : 0.3 }}
                onClick={() => apply(type, id)}
                disabled={sel === cur}
              >Apply</button>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function roleColor(role: string) {
  return role === 'admin' ? '#7c3aed' : role === 'creator' ? '#16a34a' : role === 'viewer' ? '#1e3a5f' : '#333'
}

// =============================================================================
// Callback page
// =============================================================================
function Callback() {
  const [status, setStatus] = useState('Completing login…')
  useEffect(() => {
    const p = new URLSearchParams(window.location.search)
    const error = p.get('error')
    if (error) { setStatus(`Zitadel error: ${error} — ${p.get('error_description') ?? ''}`); return }
    const code = p.get('code')
    if (!code) { setStatus(`No code. URL: ${window.location.href}`); return }
    fetch('/auth/token', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'include',
      body: JSON.stringify({ code, state: p.get('state') }),
    })
      .then(r => r.json())
      .then(data => {
        if (data.access_token) {
          localStorage.setItem('access_token', data.access_token)
          localStorage.setItem('refresh_token', data.refresh_token ?? '')
          window.location.href = '/'
        } else {
          setStatus('Login failed — ' + (data.error ?? 'unknown'))
        }
      })
      .catch(() => setStatus('Login failed — network error'))
  }, [])
  return <p style={{ padding: 32 }}>{status}</p>
}

// =============================================================================
// Home page
// =============================================================================
function Home() {
  const token = localStorage.getItem('access_token')
  const user  = token ? decodeJWT(token) : null
  const groups = [...new Set(ENDPOINTS.map(e => e.group))]

  function logout() {
    localStorage.removeItem('access_token')
    localStorage.removeItem('refresh_token')
    window.location.href = '/'
  }

  return (
    <div style={s.page}>
      {/* Header */}
      <div style={s.header}>
        <span style={s.title}>API Tester</span>
        {user ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <span style={{ color: '#aaa', fontSize: 13 }}>{String(user.name ?? user.email ?? user.sub)}</span>
            <button style={s.logoutBtn} onClick={logout}>Logout</button>
          </div>
        ) : (
          <button style={s.loginBtn} onClick={() => { window.location.href = '/auth/login' }}>
            Login / Register
          </button>
        )}
      </div>

      {!user && <p style={s.hint}>Log in to send authenticated requests.</p>}

      {/* Permissions panel */}
      <PermissionsPanel token={token} />

      {/* Endpoint tester */}
      <div style={s.tester}>
        {groups.map(group => (
          <div key={group} style={s.group}>
            <div style={s.groupLabel}>{group}</div>
            {ENDPOINTS.filter(e => e.group === group).map(ep => (
              <EndpointRow key={ep.label} ep={ep} token={token} />
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

// =============================================================================
// Root
// =============================================================================
export default function App() {
  if (window.location.pathname === '/auth/callback') return <Callback />
  return <Home />
}

// =============================================================================
// Styles
// =============================================================================
const s: Record<string, React.CSSProperties> = {
  page:     { minHeight: '100vh', width: '100%', background: '#111', color: '#e5e5e5', fontFamily: 'monospace', boxSizing: 'border-box', paddingBottom: 48 },
  header:   { display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 24px', borderBottom: '1px solid #222', background: '#1a1a1a', position: 'sticky', top: 0, zIndex: 10 },
  title:    { fontSize: 18, fontWeight: 700, color: '#fff' },
  loginBtn: { background: '#646cff', color: '#fff', border: 'none', borderRadius: 6, padding: '6px 16px', cursor: 'pointer', fontSize: 13 },
  logoutBtn:{ background: '#2a2a2a', color: '#aaa', border: '1px solid #333', borderRadius: 6, padding: '4px 12px', cursor: 'pointer', fontSize: 12 },
  hint:     { padding: '12px 24px', color: '#666', fontSize: 13, margin: 0 },
  tester:   { padding: '16px 24px', display: 'flex', flexDirection: 'column', gap: 24 },
  group:    { display: 'flex', flexDirection: 'column', gap: 4 },
  groupLabel:{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase' as const, letterSpacing: 2, color: '#555', marginBottom: 4, paddingLeft: 4 },
  row:      { display: 'flex', alignItems: 'center', gap: 10, background: '#1a1a1a', border: '1px solid #222', borderRadius: 6, padding: '8px 12px', flexWrap: 'wrap' as const },
  rowLeft:  { display: 'flex', alignItems: 'center', gap: 10, minWidth: 280, flex: '0 0 auto' },
  badge:    { fontSize: 11, fontWeight: 700, width: 50, display: 'inline-block' },
  path:     { fontSize: 13, color: '#ccc' },
  hitBtn:   { background: '#2a2a2a', color: '#fff', border: '1px solid #444', borderRadius: 4, padding: '4px 14px', cursor: 'pointer', fontSize: 12, flex: '0 0 auto' },
  resultBox:{ flex: 1, display: 'flex', alignItems: 'center', gap: 6, minWidth: 0, overflow: 'hidden' },
  bodyText: { fontSize: 12, color: '#888', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' as const },

  // permissions panel
  permPanel:  { margin: '16px 24px 0', background: '#1a1a1a', border: '1px solid #222', borderRadius: 8, overflow: 'hidden' },
  permHeader: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 14px', borderBottom: '1px solid #222' },
  permGrid:   { display: 'flex', flexDirection: 'column' as const, gap: 0 },
  permRow:    { display: 'flex', alignItems: 'center', gap: 10, padding: '7px 14px', borderBottom: '1px solid #1e1e1e', flexWrap: 'wrap' as const },
  permResource:{ fontSize: 13, color: '#ccc', minWidth: 140 },
  roleBadge:  { fontSize: 11, fontWeight: 700, padding: '2px 8px', borderRadius: 4, color: '#fff', minWidth: 48, textAlign: 'center' as const },
  select:     { background: '#222', color: '#ccc', border: '1px solid #333', borderRadius: 4, padding: '3px 6px', fontSize: 12, cursor: 'pointer' },
  grantAllBtn:{ background: '#1a3a1a', color: '#4ade80', border: '1px solid #2d5a2d', borderRadius: 4, padding: '4px 12px', cursor: 'pointer', fontSize: 12 },
}
