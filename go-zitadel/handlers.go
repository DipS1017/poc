package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// =============================================================================
// Response helpers
// =============================================================================

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// =============================================================================
// Health / Me
// =============================================================================

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sub":      user.Sub,
		"name":     user.Name,
		"email":    user.Email,
		"username": user.Username,
		"roles":    user.Roles,
	})
}

// =============================================================================
// Mock data
// =============================================================================

var mockVideos = map[string]map[string]interface{}{
	"v1": {"id": "v1", "title": "Go Concurrency Patterns", "channel_id": "c1"},
	"v2": {"id": "v2", "title": "Zitadel Auth Deep Dive", "channel_id": "c1"},
}

var mockChannels = map[string]map[string]interface{}{
	"c1": {"id": "c1", "name": "Go Academy"},
}

// =============================================================================
// Videos
// Viewer rule: any authenticated user (user platform role) can read all videos.
// Write rule:  only the creator of that specific video can edit/delete it.
// =============================================================================

func handleListVideos(w http.ResponseWriter, r *http.Request) {
	var result []map[string]interface{}
	for _, v := range mockVideos {
		result = append(result, v)
	}
	writeJSON(w, http.StatusOK, result)
}

func handleGetVideo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	v, ok := mockVideos[id]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("video %s not found", id))
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func handleUploadVideo(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	newID := fmt.Sprintf("v%d", len(mockVideos)+1)
	mockVideos[newID] = map[string]interface{}{"id": newID, "title": "New Video"}
	grantAccess(user.Sub, "video", newID, "creator")
	writeJSON(w, http.StatusCreated, map[string]string{"id": newID, "status": "uploaded"})
}

func handleUpdateVideo(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	id := r.PathValue("id")
	if !canAccess(user.Sub, "video", id, "creator") {
		writeError(w, http.StatusForbidden, "only the creator can update this video")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
}

func handleDeleteVideo(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	id := r.PathValue("id")
	if !canAccess(user.Sub, "video", id, "creator") {
		writeError(w, http.StatusForbidden, "only the creator can delete this video")
		return
	}
	delete(mockVideos, id)
	revokeAccess(user.Sub, "video", id)
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}

// =============================================================================
// Channels
// All operations require creator role — only the channel owner can see it.
// List returns only channels the current user is creator of.
// =============================================================================

func handleListChannels(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	var result []map[string]interface{}
	for id, c := range mockChannels {
		if canAccess(user.Sub, "channel", id, "creator") {
			result = append(result, c)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func handleGetChannel(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	id := r.PathValue("id")
	if !canAccess(user.Sub, "channel", id, "creator") {
		writeError(w, http.StatusForbidden, "only the channel creator can access this channel")
		return
	}
	c, ok := mockChannels[id]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("channel %s not found", id))
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	newID := fmt.Sprintf("c%d", len(mockChannels)+1)
	mockChannels[newID] = map[string]interface{}{"id": newID, "name": "New Channel"}
	grantAccess(user.Sub, "channel", newID, "creator")
	writeJSON(w, http.StatusCreated, map[string]string{"id": newID, "status": "created"})
}

func handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	id := r.PathValue("id")
	if !canAccess(user.Sub, "channel", id, "creator") {
		writeError(w, http.StatusForbidden, "only the channel creator can update this channel")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
}

func handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	id := r.PathValue("id")
	if !canAccess(user.Sub, "channel", id, "creator") {
		writeError(w, http.StatusForbidden, "only the channel creator can delete this channel")
		return
	}
	delete(mockChannels, id)
	revokeAccess(user.Sub, "channel", id)
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}

// =============================================================================
// Admin
// =============================================================================

func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]interface{}{
		{"id": "u1", "username": "alice", "role": "user"},
		{"id": "u2", "username": "bob", "role": "user"},
	})
}

func handleDeleteAdminUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}

// =============================================================================
// Debug — inspect and edit the current user's resource permissions
// =============================================================================

func handleDebugPerms(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	result := map[string]string{}
	for k, role := range permStore {
		parts := strings.SplitN(k, "|", 3)
		if len(parts) == 3 && parts[0] == user.Sub {
			result[parts[1]+"/"+parts[2]] = role
		}
	}
	writeJSON(w, http.StatusOK, result)
}

type grantRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Role         string `json:"role"` // "admin" | "creator" | "viewer"
}

func handleDebugGrant(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	var req grantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ResourceType == "" || req.ResourceID == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "body must include resource_type, resource_id, role")
		return
	}
	if _, ok := roleRank[req.Role]; !ok {
		writeError(w, http.StatusBadRequest, "role must be admin, creator, or viewer")
		return
	}
	grantAccess(user.Sub, req.ResourceType, req.ResourceID, req.Role)
	writeJSON(w, http.StatusOK, map[string]string{
		"resource": req.ResourceType + "/" + req.ResourceID,
		"role":     req.Role,
		"status":   "granted",
	})
}

func handleDebugRevoke(w http.ResponseWriter, r *http.Request) {
	user, _ := getUserFromContext(r)
	var req struct {
		ResourceType string `json:"resource_type"`
		ResourceID   string `json:"resource_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ResourceType == "" || req.ResourceID == "" {
		writeError(w, http.StatusBadRequest, "body must include resource_type and resource_id")
		return
	}
	revokeAccess(user.Sub, req.ResourceType, req.ResourceID)
	writeJSON(w, http.StatusOK, map[string]string{
		"resource": req.ResourceType + "/" + req.ResourceID,
		"status":   "revoked",
	})
}
