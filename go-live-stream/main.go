package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go-live-stream/internal/handlers"
	"go-live-stream/internal/hls"
	"go-live-stream/internal/hub"
)

func main() {
	// Load .env if present (ignored in production if file doesn't exist)
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	corsOrigins := os.Getenv("CORS_ORIGINS")
	if corsOrigins == "" {
		corsOrigins = "*"
	}
	allowedOrigins := strings.Split(corsOrigins, ",")

	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: allowedOrigins,
		AllowHeaders: []string{"*"},
		AllowMethods: []string{
			http.MethodGet, http.MethodPost,
			http.MethodDelete, http.MethodOptions,
		},
	}))

	// HLS manager — creates a temp dir and wraps ffmpeg
	hlsManager, err := hls.NewManager()
	if err != nil {
		log.Fatal(err)
	}
	defer hlsManager.Cleanup()

	streamHub := hub.NewStreamHub()
	chatHub := hub.NewChatHub()

	roomHandler := handlers.NewRoomHandler(streamHub)
	ingestHandler := handlers.NewIngestHandler(hlsManager, streamHub)
	chatHandler := handlers.NewChatHandler(chatHub, streamHub)

	// ---- REST API ----
	api := e.Group("/api")
	api.POST("/rooms", roomHandler.CreateRoom)
	api.GET("/rooms", roomHandler.ListRooms)
	api.GET("/rooms/:id", roomHandler.GetRoom)
	api.DELETE("/rooms/:id", roomHandler.DeleteRoom)

	// Publisher ingest (browser → server → ffmpeg)
	api.POST("/rooms/:id/stream/start", ingestHandler.Start)
	api.POST("/rooms/:id/stream/chunk", ingestHandler.Chunk)
	api.POST("/rooms/:id/stream/end", ingestHandler.End)

	// ---- WebSocket (chat only) ----
	e.GET("/ws/chat/:roomID", chatHandler.HandleChat)

	// ---- HLS delivery (server → viewers via HTTP) ----
	// Serve the temp dir where ffmpeg writes .m3u8 + .ts segment files.
	// hls.js fetches these as plain HTTP GET requests.
	e.Static("/hls", hlsManager.BaseDir)

	// ---- React SPA ----
	e.Static("/assets", "frontend/dist/assets")
	e.GET("/", func(c echo.Context) error { return c.File("frontend/dist/index.html") })
	e.GET("/*", func(c echo.Context) error { return c.File("frontend/dist/index.html") })

	e.Logger.Fatal(e.Start(":" + port))
}
