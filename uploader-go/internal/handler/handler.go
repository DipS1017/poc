package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.io/webpoint-solutions-llc/uploader/internal/service"
	"github.io/webpoint-solutions-llc/uploader/internal/types"
)

type Handler struct {
	s3 *service.S3Service
}

func New(s3 *service.S3Service) *Handler {
	return &Handler{s3: s3}
}

// CORS middleware
func (h *Handler) enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Expose-Headers", "ETag")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// POST /api/v1/initiate
func (h *Handler) InitiateUpload(w http.ResponseWriter, r *http.Request) {
	var req types.InitiateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Generate unique key: raw/<uuid>/<filename>
	key := fmt.Sprintf("raw/%s/%s", uuid.New().String(), req.FileName)

	uploadID, err := h.s3.InitiateUpload(r.Context(), key, req.ContentType)
	if err != nil {
		log.Printf("Failed to initiate upload: %v", err)
		h.jsonError(w, "Failed to initiate upload", http.StatusInternalServerError)
		return
	}

	partSize := h.s3.GetPartSize()
	totalParts := int((req.FileSize + partSize - 1) / partSize)

	h.jsonResponse(w, types.InitiateResponse{
		UploadID:   uploadID,
		Key:        key,
		PartSize:   partSize,
		TotalParts: totalParts,
	})
}

// POST /api/v1/sign
func (h *Handler) SignPart(w http.ResponseWriter, r *http.Request) {
	var req types.SignPartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	url, err := h.s3.SignPart(r.Context(), req.Key, req.UploadID, req.PartNumber)
	if err != nil {
		log.Printf("Failed to sign part: %v", err)
		h.jsonError(w, "Failed to generate presigned URL", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, types.SignPartResponse{URL: url})
}

// GET /api/v1/uploads/{id}/parts?key=...
func (h *Handler) ListParts(w http.ResponseWriter, r *http.Request) {
	// Extract upload ID from path: /api/v1/uploads/{id}/parts
	path := r.URL.Path
	parts := strings.Split(path, "/")
	// Path: /api/v1/uploads/{id}/parts -> parts[4] is the ID
	if len(parts) < 6 {
		h.jsonError(w, "Invalid path", http.StatusBadRequest)
		return
	}
	uploadID := parts[4]
	key := r.URL.Query().Get("key")

	if key == "" {
		h.jsonError(w, "Missing key parameter", http.StatusBadRequest)
		return
	}

	partsList, err := h.s3.ListParts(r.Context(), key, uploadID)
	if err != nil {
		log.Printf("Failed to list parts: %v", err)
		h.jsonError(w, "Failed to list parts", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, types.ListPartsResponse{Parts: partsList})
}

// POST /api/v1/complete
func (h *Handler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	var req types.CompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	location, err := h.s3.CompleteUpload(r.Context(), req.Key, req.UploadID, req.Parts)
	if err != nil {
		log.Printf("Failed to complete upload: %v", err)
		h.jsonError(w, "Failed to complete upload", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, types.CompleteResponse{
		Location: location,
		Key:      req.Key,
	})
}

// DELETE /api/v1/uploads/{id}?key=...
func (h *Handler) AbortUpload(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	parts := strings.Split(path, "/")
	if len(parts) < 5 {
		h.jsonError(w, "Invalid path", http.StatusBadRequest)
		return
	}
	uploadID := parts[4]
	key := r.URL.Query().Get("key")

	if key == "" {
		h.jsonError(w, "Missing key parameter", http.StatusBadRequest)
		return
	}

	if err := h.s3.AbortUpload(r.Context(), key, uploadID); err != nil {
		log.Printf("Failed to abort upload: %v", err)
		h.jsonError(w, "Failed to abort upload", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RegisterRoutes sets up all routes with CORS
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/initiate", h.enableCORS(h.InitiateUpload))
	mux.HandleFunc("/api/v1/sign", h.enableCORS(h.SignPart))
	mux.HandleFunc("/api/v1/uploads/", h.enableCORS(h.routeUploads))
	mux.HandleFunc("/api/v1/complete", h.enableCORS(h.CompleteUpload))
}

// routeUploads handles /api/v1/uploads/* paths
func (h *Handler) routeUploads(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/parts") && r.Method == "GET" {
		h.ListParts(w, r)
		return
	}
	if r.Method == "DELETE" {
		h.AbortUpload(w, r)
		return
	}
	h.jsonError(w, "Not found", http.StatusNotFound)
}

func (h *Handler) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(types.ErrorResponse{Error: message})
}
