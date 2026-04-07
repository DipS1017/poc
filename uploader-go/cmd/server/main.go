package main

import (
    "log"
    "net/http"

    "github.io/webpoint-solutions-llc/uploader/internal/config"
    "github.io/webpoint-solutions-llc/uploader/internal/handler"
    "github.io/webpoint-solutions-llc/uploader/internal/service"
)

func main() {
    // Load configuration
    cfg := config.Load()
    log.Printf("Starting server with MinIO endpoint: %s, bucket: %s", cfg.MinioEndpoint, cfg.MinioBucket)

    // Initialize S3 service
    s3Service, err := service.NewS3Service(cfg)
    if err != nil {
        log.Fatalf("Failed to initialize S3 service: %v", err)
    }

    // Setup handlers
    h := handler.New(s3Service)
    mux := http.NewServeMux()
    h.RegisterRoutes(mux)

    // Serve static files from web/ directory
    fs := http.FileServer(http.Dir("web"))
    mux.Handle("/", fs)

    // Start server
    addr := ":" + cfg.ServerPort
    log.Printf("Server listening on %s", addr)
    if err := http.ListenAndServe(addr, mux); err != nil {
        log.Fatalf("Server failed: %v", err)
    }
}
