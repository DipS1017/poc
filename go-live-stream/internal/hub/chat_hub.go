package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go-live-stream/internal/models"
)

const (
	writeWait        = 10 * time.Second
	pongWait         = 60 * time.Second
	pingPeriod       = (pongWait * 9) / 10
	chatSendBufSize  = 256
)

// ChatClient represents one connected chat user.
type ChatClient struct {
	id       string
	conn     *websocket.Conn
	send     chan []byte
	roomID   string
	username string
	done     chan struct{}
}

func newChatClient(conn *websocket.Conn, roomID, username string) *ChatClient {
	return &ChatClient{
		id:       uuid.New().String(),
		conn:     conn,
		send:     make(chan []byte, chatSendBufSize),
		roomID:   roomID,
		username: username,
		done:     make(chan struct{}),
	}
}

func (c *ChatClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		close(c.done)
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("[chat] write error for client %s: %v", c.id[:8], err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ChatRoom holds all connected users in a room.
type ChatRoom struct {
	id      string
	clients map[string]*ChatClient
	history []models.ChatMessage // last N messages for new joiners
	mu      sync.RWMutex
}

const maxChatHistory = 50

func (r *ChatRoom) addClient(client *ChatClient) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[client.id] = client
}

func (r *ChatRoom) removeClient(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, clientID)
}

func (r *ChatRoom) broadcast(msg models.ChatMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	r.mu.Lock()
	// Store in history
	r.history = append(r.history, msg)
	if len(r.history) > maxChatHistory {
		r.history = r.history[len(r.history)-maxChatHistory:]
	}

	for _, client := range r.clients {
		select {
		case client.send <- data:
		default:
			log.Printf("[chat] send buffer full for client %s, dropping message", client.id[:8])
		}
	}
	r.mu.Unlock()
}

func (r *ChatRoom) getHistory() []models.ChatMessage {
	r.mu.RLock()
	defer r.mu.RUnlock()
	hist := make([]models.ChatMessage, len(r.history))
	copy(hist, r.history)
	return hist
}

// ChatHub manages all chat rooms.
type ChatHub struct {
	rooms map[string]*ChatRoom
	mu    sync.RWMutex
}

func NewChatHub() *ChatHub {
	return &ChatHub{
		rooms: make(map[string]*ChatRoom),
	}
}

func (h *ChatHub) getOrCreateRoom(roomID string) *ChatRoom {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[roomID]; ok {
		return room
	}
	room := &ChatRoom{
		id:      roomID,
		clients: make(map[string]*ChatClient),
	}
	h.rooms[roomID] = room
	return room
}

// HandleChat manages the full lifecycle of a chat WebSocket connection.
func (h *ChatHub) HandleChat(conn *websocket.Conn, roomID, username string) {
	room := h.getOrCreateRoom(roomID)
	client := newChatClient(conn, roomID, username)

	go client.writePump()
	room.addClient(client)

	log.Printf("[chat] %q joined room %s", username, roomID)

	// Send recent history to new joiner
	history := room.getHistory()
	for _, msg := range history {
		data, _ := json.Marshal(msg)
		select {
		case client.send <- data:
		default:
		}
	}

	// Broadcast join message
	room.broadcast(models.ChatMessage{
		ID:        uuid.New().String(),
		RoomID:    roomID,
		Username:  username,
		Content:   username + " joined the stream",
		Type:      "join",
		Timestamp: time.Now(),
	})

	defer func() {
		room.removeClient(client.id)
		close(client.send)
		<-client.done

		// Broadcast leave message
		leaveMsg := models.ChatMessage{
			ID:        uuid.New().String(),
			RoomID:    roomID,
			Username:  username,
			Content:   username + " left the stream",
			Type:      "leave",
			Timestamp: time.Now(),
		}
		data, _ := json.Marshal(leaveMsg)
		room.mu.RLock()
		for _, c := range room.clients {
			select {
			case c.send <- data:
			default:
			}
		}
		room.mu.RUnlock()

		log.Printf("[chat] %q left room %s", username, roomID)
	}()

	conn.SetReadLimit(4096)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	type incomingMsg struct {
		Content string `json:"content"`
	}

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		conn.SetReadDeadline(time.Now().Add(pongWait))

		var incoming incomingMsg
		if err := json.Unmarshal(data, &incoming); err != nil {
			continue
		}

		content := incoming.Content
		if len(content) == 0 || len(content) > 500 {
			continue
		}

		room.broadcast(models.ChatMessage{
			ID:        uuid.New().String(),
			RoomID:    roomID,
			Username:  username,
			Content:   content,
			Type:      "message",
			Timestamp: time.Now(),
		})
	}
}
