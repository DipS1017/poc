import { useRef } from 'react'
import { useStreamViewer } from '../hooks/useStreamViewer.js'

export default function VideoPlayer({ roomID }) {
  const videoRef = useRef(null)
  const { isStreaming, error } = useStreamViewer(videoRef, roomID)

  return (
    <div className="video-container">
      <div className="video-wrapper">
        <video
          ref={videoRef}
          className="video-player"
          autoPlay
          muted
          playsInline
          controls
        />
        {!isStreaming && (
          <div className="video-overlay">
            <div className="offline-placeholder">
              <div className="offline-icon">📺</div>
              <p>Waiting for stream to start...</p>
              <p className="hint">The page will update automatically.</p>
            </div>
          </div>
        )}
      </div>

      <div className="video-meta">
        {isStreaming && <span className="live-badge">LIVE</span>}
        {error && <span className="video-error">{error}</span>}
        {isStreaming && (
          <span className="hint-meta">Start audio: unmute via the video controls</span>
        )}
      </div>
    </div>
  )
}
