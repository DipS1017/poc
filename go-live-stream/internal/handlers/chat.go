package handlers

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"go-live-stream/internal/hub"
)

var chatUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type ChatHandler struct {
	chatHub   *hub.ChatHub
	streamHub *hub.StreamHub
}

func NewChatHandler(ch *hub.ChatHub, sh *hub.StreamHub) *ChatHandler {
	return &ChatHandler{chatHub: ch, streamHub: sh}
}

func (h *ChatHandler) HandleChat(c echo.Context) error {
	roomID := c.Param("roomID")
	username := c.QueryParam("username")
	if username == "" {
		username = "Anonymous"
	}

	if _, ok := h.streamHub.GetRoomInfo(roomID); !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
	}

	conn, err := chatUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}

	h.chatHub.HandleChat(conn, roomID, username)
	return nil
}
