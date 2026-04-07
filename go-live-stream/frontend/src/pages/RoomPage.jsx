import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { getRoom } from '../api/rooms.js'
import VideoStreamer from '../components/VideoStreamer.jsx'
import VideoPlayer from '../components/VideoPlayer.jsx'
import Chat from '../components/Chat.jsx'

export default function RoomPage() {
  const { id } = useParams()
  const navigate = useNavigate()

  const [room, setRoom] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  const username = localStorage.getItem('username') || 'Viewer'
  const ownedRooms = JSON.parse(localStorage.getItem('ownedRooms') || '{}')
  const isHost = !!ownedRooms[id]

  useEffect(() => {
    getRoom(id)
      .then(setRoom)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  if (loading) return <div className="page loading-page">Loading room...</div>
  if (error) {
    return (
      <div className="page error-page">
        <p>Room not found or server error: {error}</p>
        <button className="btn btn-primary" onClick={() => navigate('/')}>
          Back to Home
        </button>
      </div>
    )
  }

  return (
    <div className="page room-page">
      <header className="room-header">
        <button className="btn-back" onClick={() => navigate('/')}>
          ← GoStream
        </button>

        <div className="room-info">
          <h1 className="room-title">{room.title}</h1>
          {room.description && <p className="room-desc">{room.description}</p>}
          <div className="room-meta">
            <span>by {room.hostName}</span>
            {isHost && <span className="host-badge">You are the host</span>}
          </div>
        </div>

        <div className="room-header-right">
          {room.isLive && <span className="live-badge">LIVE</span>}
        </div>
      </header>

      <div className="room-layout">
        <div className="room-main">
          {isHost ? (
            <VideoStreamer roomID={id} username={username} />
          ) : (
            <VideoPlayer roomID={id} username={username} />
          )}
        </div>

        <aside className="room-sidebar">
          <Chat roomID={id} username={username} />
        </aside>
      </div>
    </div>
  )
}
