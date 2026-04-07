# GoStream — Live Streaming POC

A full-stack live streaming + live chat POC using Go (Echo) + React.

## Architecture

### Backend (`main.go` + `internal/`)

**Framework**: Echo v4
**WebSocket library**: gorilla/websocket
**Pattern**: One goroutine per connection, mutex-protected shared state (no channels between rooms)

#### Key packages

| Package | Purpose |
|---|---|
| `internal/models` | Shared data types (RoomInfo, ChatMessage, StreamSignal) |
| `internal/hub/stream_hub.go` | Manages stream rooms; publisher→viewer binary relay |
| `internal/hub/chat_hub.go` | Manages chat rooms; broadcast with history |
| `internal/handlers/` | Echo HTTP/WebSocket handlers (thin wrappers around hubs) |

#### WebSocket protocols

**Stream WS** `/ws/stream/:roomID?role=publisher|viewer&username=...`

- Publisher sends **text** (JSON signals) and **binary** (raw MediaRecorder chunks)
- Viewer receives **binary** (video chunks) and **text** (signals)

Signal types:
- `stream_start` — publisher → server → viewers. Payload: `{ mimeType: string }`
- `stream_end` — publisher → server → viewers
- `already_live` — server → new viewer (when joining an active stream). Payload: `{ mimeType: string }`
- `viewer_count` — server → publisher. Payload: `{ count: int }`
- `error` — server → client. Payload: `{ message: string }`

**Chat WS** `/ws/chat/:roomID?username=...`

- Client sends: `{ "content": "hello" }`
- Server sends: `ChatMessage` JSON objects
- Message types: `"message"`, `"join"`, `"leave"`
- Last 50 messages are cached and replayed to new joiners

#### Late-viewer strategy (stream_hub.go)

When a viewer joins an active stream, the server:
1. Sends `already_live` signal with mimeType
2. Sends the last 60 cached binary chunks (≈6s at 100ms intervals)

This lets late-joiners decode WebM without needing the stream from the beginning.

#### REST API

| Method | Path | Description |
|---|---|---|
| POST | `/api/rooms` | Create room |
| GET | `/api/rooms` | List all rooms |
| GET | `/api/rooms/:id` | Get room info |
| DELETE | `/api/rooms/:id` | Delete room (closes all connections) |

Rooms persist in memory (no database). Restart = empty state.

### Frontend (`frontend/`)

**Framework**: React 18 + React Router v6
**Bundler**: Vite 5
**Styling**: Vanilla CSS (dark theme, Twitch-inspired)
**No external UI library**

#### Key files

| File | Purpose |
|---|---|
| `src/hooks/useStreamPublisher.js` | Camera/screen capture → MediaRecorder → WebSocket |
| `src/hooks/useStreamViewer.js` | WebSocket binary → MediaSource → `<video>` |
| `src/hooks/useChat.js` | Chat WebSocket with deduplication |
| `src/components/VideoStreamer.jsx` | Host UI: preview + go live controls |
| `src/components/VideoPlayer.jsx` | Viewer UI: video element + status overlay |
| `src/components/Chat.jsx` | Chat panel with auto-scroll |
| `src/pages/Home.jsx` | Room list + username setup |
| `src/pages/RoomPage.jsx` | Stream room (auto-detects host vs viewer via localStorage) |

#### Host detection

`localStorage.ownedRooms` (`Record<roomID, true>`) is set when a user creates a room.
`RoomPage` reads this to show `VideoStreamer` (host) or `VideoPlayer` (viewer).
This is session-scoped — no auth system.

#### Video streaming (MediaSource Extensions)

Publisher side (`useStreamPublisher.js`):
1. `navigator.mediaDevices.getUserMedia` / `getDisplayMedia`
2. `MediaRecorder` (VP9/VP8 WebM, 100ms timeslice, 2.5Mbps target)
3. `ondataavailable` → send binary via WebSocket

Viewer side (`useStreamViewer.js`):
1. `MediaSource` + `SourceBuffer` with `mode = 'sequence'`
2. Incoming binary chunks are queued and appended serially (SourceBuffer is not re-entrant)
3. `QuotaExceededError` → evict data before `currentTime - 10s`

**Browser support**: Chrome/Edge (full). Firefox (partial — VP8 WebM works). Safari — NOT supported (MSE + MediaRecorder codec mismatch).

## Running the project

```bash
# Install dependencies
make install

# Dev mode (backend :8080, frontend :5173 with proxy)
make dev

# Production build + serve (both on :8080)
make build && make run
```

## Dev notes

- Vite proxies `/api` and `/ws` to `:8080` in dev mode — no CORS issues
- Backend serves `frontend/dist/` in production (SPA fallback on `/*`)
- Stream state is in-memory: restarting the server drops all rooms/streams
- The 100ms MediaRecorder timeslice gives ~100ms latency overhead on top of network RTT
- WebSocket send buffers are sized at 512 msgs for stream clients; slow viewers drop chunks rather than blocking the publisher

## Known limitations

- No authentication (username is localStorage only)
- No persistence (rooms lost on restart)
- Safari not supported for streaming (use Chrome or Edge)
- One publisher per room (no multi-host)
- No recording/VOD
- No adaptive bitrate
