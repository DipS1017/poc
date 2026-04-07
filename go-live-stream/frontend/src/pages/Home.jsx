import { useState } from 'react'
import RoomList from '../components/RoomList.jsx'
import CreateRoom from '../components/CreateRoom.jsx'

export default function Home() {
  const [showCreate, setShowCreate] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)
  const [username, setUsername] = useState(localStorage.getItem('username') || '')
  const [editingName, setEditingName] = useState(!localStorage.getItem('username'))

  const handleSaveName = (e) => {
    e.preventDefault()
    const trimmed = username.trim()
    if (!trimmed) return
    localStorage.setItem('username', trimmed)
    setEditingName(false)
  }

  const handleCreateClose = () => {
    setShowCreate(false)
    setRefreshKey((k) => k + 1)
  }

  return (
    <div className="page home-page">
      <header className="site-header">
        <div className="header-brand">
          <span className="brand-icon">📺</span>
          <span className="brand-name">GoStream</span>
        </div>

        <div className="header-actions">
          {editingName ? (
            <form onSubmit={handleSaveName} className="username-form">
              <input
                type="text"
                placeholder="Your username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                maxLength={30}
                autoFocus
              />
              <button type="submit" className="btn btn-sm btn-primary">Save</button>
            </form>
          ) : (
            <div className="username-display">
              <span className="avatar">{username[0]?.toUpperCase()}</span>
              <span>{username}</span>
              <button
                className="btn-link"
                onClick={() => setEditingName(true)}
              >
                edit
              </button>
            </div>
          )}

          {!editingName && (
            <button
              className="btn btn-primary"
              onClick={() => setShowCreate(true)}
            >
              Go Live
            </button>
          )}
        </div>
      </header>

      <main className="home-main">
        <div className="section-header">
          <h2>Live Channels</h2>
          <button className="btn-link" onClick={() => setRefreshKey((k) => k + 1)}>
            Refresh
          </button>
        </div>

        <RoomList refreshKey={refreshKey} />
      </main>

      {showCreate && <CreateRoom onCancel={handleCreateClose} />}
    </div>
  )
}
