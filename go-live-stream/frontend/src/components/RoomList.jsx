import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { listRooms, deleteRoom } from '../api/rooms.js'

export default function RoomList({ refreshKey }) {
  const navigate = useNavigate()
  const [rooms, setRooms] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  const ownedRooms = JSON.parse(localStorage.getItem('ownedRooms') || '{}')

  const fetchRooms = async () => {
    try {
      const data = await listRooms()
      setRooms(data)
      setError(null)
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchRooms()
    const interval = setInterval(fetchRooms, 5000) // poll every 5s
    return () => clearInterval(interval)
  }, [refreshKey])

  const handleDelete = async (e, id) => {
    e.stopPropagation()
    if (!window.confirm('Delete this room?')) return
    try {
      await deleteRoom(id)
      const owned = JSON.parse(localStorage.getItem('ownedRooms') || '{}')
      delete owned[id]
      localStorage.setItem('ownedRooms', JSON.stringify(owned))
      setRooms((r) => r.filter((room) => room.id !== id))
    } catch (e) {
      alert('Failed to delete: ' + e.message)
    }
  }

  if (loading) return <div className="loading">Loading rooms...</div>
  if (error) return <div className="error-msg">Error: {error}</div>
  if (rooms.length === 0) {
    return (
      <div className="empty-state">
        <p>No streams available. Create one to get started!</p>
      </div>
    )
  }

  return (
    <div className="room-grid">
      {rooms.map((room) => (
        <div
          key={room.id}
          className="room-card"
          onClick={() => navigate(`/room/${room.id}`)}
        >
          <div className="room-card-thumb">
            {room.isLive ? (
              <div className="thumb-live">
                <span className="live-dot" />
                LIVE
              </div>
            ) : (
              <div className="thumb-offline">OFFLINE</div>
            )}
          </div>

          <div className="room-card-body">
            <h3 className="room-title">{room.title}</h3>
            {room.description && (
              <p className="room-desc">{room.description}</p>
            )}
            <div className="room-meta">
              <span className="room-host">by {room.hostName}</span>
              {room.isLive && (
                <span className="room-viewers">{room.viewerCount} watching</span>
              )}
            </div>
          </div>

          {ownedRooms[room.id] && (
            <button
              className="room-delete"
              onClick={(e) => handleDelete(e, room.id)}
              title="Delete room"
            >
              ✕
            </button>
          )}
        </div>
      ))}
    </div>
  )
}
