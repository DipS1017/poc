import { useRef, useEffect, useState, useCallback } from 'react'

const WS_BASE = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
const WS_HOST = import.meta.env.DEV ? 'ws://localhost:8080' : `${WS_BASE}//${window.location.host}`

export function useChat(roomID, username) {
  const wsRef = useRef(null)
  const [messages, setMessages] = useState([])
  const [isConnected, setIsConnected] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    const ws = new WebSocket(
      `${WS_HOST}/ws/chat/${roomID}?username=${encodeURIComponent(username)}`
    )
    wsRef.current = ws

    ws.onopen = () => {
      setIsConnected(true)
      setError(null)
    }

    ws.onclose = () => {
      setIsConnected(false)
    }

    ws.onerror = () => {
      setError('Chat connection failed')
    }

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        setMessages((prev) => {
          // Deduplicate by ID
          if (prev.some((m) => m.id === msg.id)) return prev
          return [...prev, msg]
        })
      } catch {
        // ignore
      }
    }

    return () => {
      ws.close()
    }
  }, [roomID, username])

  const sendMessage = useCallback((content) => {
    if (!content.trim()) return
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      setError('Not connected to chat')
      return
    }
    wsRef.current.send(JSON.stringify({ content: content.trim() }))
  }, [])

  return { messages, isConnected, error, sendMessage }
}
