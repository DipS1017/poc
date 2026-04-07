package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go-live-stream/internal/hub"
	"go-live-stream/internal/models"
)

type RoomHandler struct {
	streamHub *hub.StreamHub
}

func NewRoomHandler(sh *hub.StreamHub) *RoomHandler {
	return &RoomHandler{streamHub: sh}
}

type createRoomRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	HostName    string `json:"hostName"`
}

func (h *RoomHandler) CreateRoom(c echo.Context) error {
	var req createRoomRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if req.Title == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "title is required"})
	}
	if req.HostName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "hostName is required"})
	}

	info := models.RoomInfo{
		ID:          uuid.New().String(),
		Title:       req.Title,
		Description: req.Description,
		HostName:    req.HostName,
		IsLive:      false,
		CreatedAt:   time.Now(),
	}
	h.streamHub.CreateRoom(info)
	return c.JSON(http.StatusCreated, info)
}

func (h *RoomHandler) ListRooms(c echo.Context) error {
	rooms := h.streamHub.ListRooms()
	if rooms == nil {
		rooms = []models.RoomInfo{}
	}
	return c.JSON(http.StatusOK, rooms)
}

func (h *RoomHandler) GetRoom(c echo.Context) error {
	id := c.Param("id")
	info, ok := h.streamHub.GetRoomInfo(id)
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
	}
	return c.JSON(http.StatusOK, info)
}

func (h *RoomHandler) DeleteRoom(c echo.Context) error {
	id := c.Param("id")
	if _, ok := h.streamHub.GetRoomInfo(id); !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
	}
	h.streamHub.DeleteRoom(id)
	return c.NoContent(http.StatusNoContent)
}
