import { useRef, useEffect, useState } from 'react'
import Hls from 'hls.js'

export function useStreamViewer(videoRef, roomID) {
  const hlsRef = useRef(null)
  const [isStreaming, setIsStreaming] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    const playlistUrl = `/hls/${roomID}/index.m3u8`

    // Safari has native HLS support — no need for hls.js
    if (!Hls.isSupported()) {
      if (video.canPlayType('application/vnd.apple.mpegurl')) {
        video.src = playlistUrl
        video.addEventListener('loadedmetadata', () => {
          setIsStreaming(true)
          video.play().catch(() => {})
        }, { once: true })
      } else {
        setError('HLS not supported in this browser. Use Chrome, Edge, or Safari.')
      }
      return
    }

    const hls = new Hls({
      // Keep the viewer close to the live edge (within ~3 segments = ~6s)
      liveSyncDurationCount: 3,
      liveMaxLatencyDurationCount: 6,
      enableWorker: true,
      lowLatencyMode: false,
      backBufferLength: 30,

      // Tolerate small gaps between segments (e.g. from timestamp rounding)
      // without stalling playback.
      maxBufferHole: 0.5,
      maxSeekHole: 0.5,
      nudgeMaxRetry: 5,
      nudgeOffset: 0.1,

      // Retry aggressively — the stream might not have started yet when the viewer loads
      manifestLoadingMaxRetry: 120,
      manifestLoadingRetryDelay: 3000,
      levelLoadingMaxRetry: 6,
      fragLoadingMaxRetry: 6,
    })
    hlsRef.current = hls

    hls.loadSource(playlistUrl)
    hls.attachMedia(video)

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      setIsStreaming(true)
      setError(null)
      video.play().catch(() => {})
    })

    hls.on(Hls.Events.ERROR, (_, data) => {
      // Non-fatal errors (e.g. a single failed segment) are handled internally by hls.js
      if (!data.fatal) return

      switch (data.type) {
        case Hls.ErrorTypes.NETWORK_ERROR:
          // Playlist not found (404) = stream hasn't started yet → keep retrying
          hls.startLoad()
          break
        case Hls.ErrorTypes.MEDIA_ERROR:
          hls.recoverMediaError()
          break
        default:
          setError('Playback error. Try refreshing the page.')
          hls.destroy()
      }
    })

    // When the stream ends ffmpeg closes the playlist with #EXT-X-ENDLIST.
    // hls.js fires BUFFER_EOS which causes the video to naturally pause at the end.
    // We detect this by watching for the video's ended event.
    const handleEnded = () => setIsStreaming(false)
    video.addEventListener('ended', handleEnded)

    return () => {
      hls.destroy()
      video.removeEventListener('ended', handleEnded)
    }
  }, [roomID, videoRef])

  return { isStreaming, error }
}
