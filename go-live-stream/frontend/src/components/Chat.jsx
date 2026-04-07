import { useState, useRef, useEffect } from 'react'
import { useChat } from '../hooks/useChat.js'

export default function Chat({ roomID, username }) {
  const { messages, isConnected, error, sendMessage } = useChat(roomID, username)
  const [input, setInput] = useState('')
  const bottomRef = useRef(null)

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const handleSend = (e) => {
    e.preventDefault()
    if (!input.trim()) return
    sendMessage(input)
    setInput('')
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend(e)
    }
  }

  return (
    <div className="chat-panel">
      <div className="chat-header">
        <span>Live Chat</span>
        <span className={`chat-status ${isConnected ? 'connected' : 'disconnected'}`}>
          {isConnected ? 'Connected' : 'Connecting...'}
        </span>
      </div>

      {error && <div className="chat-error">{error}</div>}

      <div className="chat-messages">
        {messages.length === 0 && (
          <div className="chat-empty">No messages yet. Say hello!</div>
        )}
        {messages.map((msg) => (
          <ChatMessage key={msg.id} msg={msg} self={msg.username === username} />
        ))}
        <div ref={bottomRef} />
      </div>

      <form className="chat-input-form" onSubmit={handleSend}>
        <input
          className="chat-input"
          type="text"
          placeholder={isConnected ? 'Send a message...' : 'Connecting...'}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={!isConnected}
          maxLength={500}
        />
        <button
          className="chat-send-btn"
          type="submit"
          disabled={!isConnected || !input.trim()}
        >
          Send
        </button>
      </form>
    </div>
  )
}

function ChatMessage({ msg, self }) {
  if (msg.type === 'join' || msg.type === 'leave') {
    return <div className="chat-event">{msg.content}</div>
  }

  return (
    <div className={`chat-msg ${self ? 'self' : ''}`}>
      <span className="chat-username">{msg.username}</span>
      <span className="chat-content">{msg.content}</span>
    </div>
  )
}
