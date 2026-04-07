package hub

import (
	"sync"

	"go-live-stream/internal/models"
)

// StreamHub is an in-memory store of room metadata.
// IsLive is kept in sync by the ingest handler via SetLive.
type StreamHub struct {
	rooms map[string]models.RoomInfo
	mu    sync.RWMutex
}

func NewStreamHub() *StreamHub {
	return &StreamHub{rooms: make(map[string]models.RoomInfo)}
}

func (h *StreamHub) CreateRoom(info models.RoomInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rooms[info.ID] = info
}

func (h *StreamHub) GetRoomInfo(id string) (models.RoomInfo, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	info, ok := h.rooms[id]
	return info, ok
}

func (h *StreamHub) ListRooms() []models.RoomInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	rooms := make([]models.RoomInfo, 0, len(h.rooms))
	for _, r := range h.rooms {
		rooms = append(rooms, r)
	}
	return rooms
}

func (h *StreamHub) DeleteRoom(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms, id)
}

// SetLive is called by the ingest handler when a stream starts or ends.
func (h *StreamHub) SetLive(id string, isLive bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if info, ok := h.rooms[id]; ok {
		info.IsLive = isLive
		h.rooms[id] = info
	}
}
