import { useRef, useCallback, useState, useEffect } from 'react'

const MIME_TYPES = [
  'video/webm;codecs=vp9,opus',
  'video/webm;codecs=vp8,opus',
  'video/webm',
]

function getSupportedMimeType() {
  for (const type of MIME_TYPES) {
    if (MediaRecorder.isTypeSupported(type)) return type
  }
  return ''
}

export function useStreamPublisher(videoRef, roomID, username) {
  const mediaRecorderRef = useRef(null)
  const streamRef = useRef(null)
  // Queue of Blob chunks waiting to be POSTed
  const chunkQueueRef = useRef([])
  // True while a POST is in-flight — ensures chunks are sent in order
  const isSendingRef = useRef(false)
  const activeRef = useRef(false)

  const [isStreaming, setIsStreaming] = useState(false)
  const [error, setError] = useState(null)
  const [mimeType, setMimeType] = useState('')

  // Send the next queued chunk. Chunks must arrive at ffmpeg in order so we
  // await each POST before sending the next one.
  const flushQueue = useCallback(async () => {
    if (isSendingRef.current || chunkQueueRef.current.length === 0) return
    isSendingRef.current = true

    while (chunkQueueRef.current.length > 0 && activeRef.current) {
      const chunk = chunkQueueRef.current.shift()
      try {
        const res = await fetch(`/api/rooms/${roomID}/stream/chunk`, {
          method: 'POST',
          body: chunk,
          headers: { 'Content-Type': 'application/octet-stream' },
        })
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
      } catch (e) {
        console.error('[publisher] chunk upload failed:', e)
        setError('Upload error: ' + e.message)
      }
    }

    isSendingRef.current = false
  }, [roomID])

  const startStream = useCallback(async (useScreen = false) => {
    setError(null)
    try {
      let mediaStream
      if (useScreen) {
        mediaStream = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: true })
      } else {
        mediaStream = await navigator.mediaDevices.getUserMedia({
          video: { width: { ideal: 1280 }, height: { ideal: 720 }, frameRate: { ideal: 30 } },
          audio: true,
        })
      }

      streamRef.current = mediaStream
      if (videoRef.current) videoRef.current.srcObject = mediaStream

      const type = getSupportedMimeType()
      if (!type) throw new Error('No supported codec found in this browser')
      setMimeType(type)

      // Notify server to launch ffmpeg
      const res = await fetch(`/api/rooms/${roomID}/stream/start`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mimeType: type }),
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        throw new Error(err.error || 'Failed to start stream on server')
      }

      activeRef.current = true
      chunkQueueRef.current = []

      const recorder = new MediaRecorder(mediaStream, {
        mimeType: type,
        videoBitsPerSecond: 2_500_000,
        audioBitsPerSecond: 128_000,
      })
      mediaRecorderRef.current = recorder

      recorder.ondataavailable = (e) => {
        if (e.data && e.data.size > 0) {
          chunkQueueRef.current.push(e.data)
          flushQueue()
        }
      }

      recorder.onstart = () => setIsStreaming(true)
      recorder.onstop = () => setIsStreaming(false)

      // 100 ms timeslice — more frequent, smaller chunks to ffmpeg.
      // Reduces the PTS burst size further and gives the encoder a
      // steadier input cadence.
      recorder.start(100)
    } catch (e) {
      if (e.name === 'NotAllowedError') {
        setError('Camera/microphone permission denied')
      } else {
        setError(e.message)
      }
    }
  }, [videoRef, roomID, flushQueue])

  const stopStream = useCallback(async () => {
    activeRef.current = false
    mediaRecorderRef.current?.stop()
    streamRef.current?.getTracks().forEach((t) => t.stop())
    streamRef.current = null
    if (videoRef.current) videoRef.current.srcObject = null
    setIsStreaming(false)
    setMimeType('')

    // Give the queue a moment to finish sending before signalling end
    await new Promise((r) => setTimeout(r, 1200))
    fetch(`/api/rooms/${roomID}/stream/end`, { method: 'POST' }).catch(console.error)
  }, [videoRef, roomID])

  // Stop cleanly when the component unmounts mid-stream
  useEffect(() => {
    return () => {
      if (activeRef.current) stopStream()
    }
  }, [stopStream])

  return { isStreaming, error, mimeType, startStream, stopStream }
}
