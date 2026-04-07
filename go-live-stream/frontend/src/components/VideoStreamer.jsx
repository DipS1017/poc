import { useRef, useState } from 'react'
import { useStreamPublisher } from '../hooks/useStreamPublisher.js'

export default function VideoStreamer({ roomID, username }) {
  const videoRef = useRef(null)
  const [useScreen, setUseScreen] = useState(false)

  const { isStreaming, error, mimeType, startStream, stopStream } =
    useStreamPublisher(videoRef, roomID, username)

  return (
    <div className="video-container streamer">
      <div className="video-wrapper">
        {/* local preview — muted so the streamer doesn't hear themselves */}
        <video ref={videoRef} className="video-player" autoPlay muted playsInline />
        {!isStreaming && (
          <div className="video-overlay">
            <div className="offline-placeholder">
              <div className="offline-icon">🎥</div>
              <p>Ready to go live</p>
            </div>
          </div>
        )}
        {isStreaming && (
          <div className="live-indicator">
            <span className="live-dot" /> LIVE
          </div>
        )}
      </div>

      <div className="streamer-controls">
        <div className="source-select">
          <label>
            <input
              type="radio"
              name="source"
              checked={!useScreen}
              onChange={() => setUseScreen(false)}
              disabled={isStreaming}
            />
            Camera
          </label>
          <label>
            <input
              type="radio"
              name="source"
              checked={useScreen}
              onChange={() => setUseScreen(true)}
              disabled={isStreaming}
            />
            Screen Share
          </label>
        </div>

        <div className="stream-actions">
          {!isStreaming ? (
            <button className="btn btn-live" onClick={() => startStream(useScreen)}>
              Go Live
            </button>
          ) : (
            <button className="btn btn-end" onClick={stopStream}>
              End Stream
            </button>
          )}
        </div>

        <div className="stream-stats">
          {isStreaming && (
            <span className="stat">
              <span className="stat-label">Encoder</span>
              <span className="stat-value codec">{mimeType.split(';')[0]} → H.264 HLS</span>
            </span>
          )}
        </div>
      </div>

      {error && <div className="stream-error">{error}</div>}
    </div>
  )
}
