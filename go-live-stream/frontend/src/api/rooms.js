const BASE = '/api'

export async function listRooms() {
  const res = await fetch(`${BASE}/rooms`)
  if (!res.ok) throw new Error('Failed to fetch rooms')
  return res.json()
}

export async function getRoom(id) {
  const res = await fetch(`${BASE}/rooms/${id}`)
  if (!res.ok) throw new Error('Room not found')
  return res.json()
}

export async function createRoom({ title, description, hostName }) {
  const res = await fetch(`${BASE}/rooms`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title, description, hostName }),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error || 'Failed to create room')
  }
  return res.json()
}

export async function deleteRoom(id) {
  const res = await fetch(`${BASE}/rooms/${id}`, { method: 'DELETE' })
  if (!res.ok) throw new Error('Failed to delete room')
}
