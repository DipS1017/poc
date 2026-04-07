import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { createRoom } from '../api/rooms.js'

export default function CreateRoom({ onCancel }) {
  const navigate = useNavigate()
  const [form, setForm] = useState({ title: '', description: '' })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  const storedUsername = localStorage.getItem('username') || ''

  const handleChange = (e) => {
    setForm((f) => ({ ...f, [e.target.name]: e.target.value }))
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setLoading(true)
    setError(null)

    const username = localStorage.getItem('username') || 'Host'

    try {
      const room = await createRoom({
        title: form.title.trim(),
        description: form.description.trim(),
        hostName: username,
      })
      // Mark this room as owned by this session
      const ownedRooms = JSON.parse(localStorage.getItem('ownedRooms') || '{}')
      ownedRooms[room.id] = true
      localStorage.setItem('ownedRooms', JSON.stringify(ownedRooms))

      navigate(`/room/${room.id}`)
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="modal-overlay" onClick={onCancel}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2>Create a Stream</h2>
          <button className="modal-close" onClick={onCancel}>✕</button>
        </div>

        {!storedUsername && (
          <div className="modal-notice">
            Set your username first on the home page.
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div className="field">
            <label>Stream Title *</label>
            <input
              name="title"
              type="text"
              value={form.title}
              onChange={handleChange}
              placeholder="My Awesome Stream"
              required
              maxLength={100}
              autoFocus
            />
          </div>

          <div className="field">
            <label>Description</label>
            <textarea
              name="description"
              value={form.description}
              onChange={handleChange}
              placeholder="What are you streaming today?"
              rows={3}
              maxLength={300}
            />
          </div>

          {error && <div className="form-error">{error}</div>}

          <div className="modal-actions">
            <button type="button" className="btn btn-secondary" onClick={onCancel}>
              Cancel
            </button>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={loading || !form.title.trim() || !storedUsername}
            >
              {loading ? 'Creating...' : 'Create Room'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
