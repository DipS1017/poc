package models

import "time"

type RoomInfo struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	HostName    string    `json:"hostName"`
	IsLive      bool      `json:"isLive"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ChatMessage struct {
	ID        string    `json:"id"`
	RoomID    string    `json:"roomId"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	Type      string    `json:"type"` // "message", "join", "leave"
	Timestamp time.Time `json:"timestamp"`
}
