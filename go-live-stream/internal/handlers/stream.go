package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"go-live-stream/internal/hls"
	"go-live-stream/internal/hub"
)

// IngestHandler handles the publisher-side HTTP ingest endpoints.
//
// Flow:
//   POST /api/rooms/:id/stream/start  — starts an ffmpeg HLS session
//   POST /api/rooms/:id/stream/chunk  — streams raw WebM bytes into ffmpeg
//   POST /api/rooms/:id/stream/end    — shuts down the ffmpeg session
type IngestHandler struct {
	hlsManager *hls.Manager
	streamHub  *hub.StreamHub
}

func NewIngestHandler(hm *hls.Manager, sh *hub.StreamHub) *IngestHandler {
	return &IngestHandler{hlsManager: hm, streamHub: sh}
}

// Start begins an HLS session for the room. Called once before the first chunk.
func (h *IngestHandler) Start(c echo.Context) error {
	roomID := c.Param("id")
	if _, ok := h.streamHub.GetRoomInfo(roomID); !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
	}

	if _, err := h.hlsManager.Start(roomID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	h.streamHub.SetLive(roomID, true)
	log.Printf("[ingest] stream started for room %s", roomID)
	return c.JSON(http.StatusOK, map[string]string{"status": "started"})
}

// Chunk accepts a raw binary body (one MediaRecorder chunk) and writes it
// to the running ffmpeg process stdin. The chunks must arrive in order.
func (h *IngestHandler) Chunk(c echo.Context) error {
	roomID := c.Param("id")

	session, ok := h.hlsManager.GetSession(roomID)
	if !ok {
		return c.JSON(http.StatusConflict, map[string]string{"error": "no active session — call /stream/start first"})
	}

	data, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "read body: " + err.Error()})
	}

	if len(data) == 0 {
		return c.JSON(http.StatusOK, map[string]string{"status": "empty"})
	}

	if _, err := session.Write(data); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "write to ffmpeg: " + err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "bytes": fmt.Sprint(len(data))})
}

// End shuts down the ffmpeg session and marks the room as offline.
func (h *IngestHandler) End(c echo.Context) error {
	roomID := c.Param("id")
	h.hlsManager.Stop(roomID)
	h.streamHub.SetLive(roomID, false)
	log.Printf("[ingest] stream ended for room %s", roomID)
	return c.JSON(http.StatusOK, map[string]string{"status": "ended"})
}
