package main

import "net/http"

func registerRoutes(mux *http.ServeMux, client *http.Client) {
	// Platform role wrappers (JWT roles from Zitadel, hierarchy applies):
	//   viewer  → can read videos
	//   creator → can write videos + access own channels  (also satisfies viewer routes)
	//   admin   → platform admin                          (also satisfies creator + viewer routes)
	auth := func(h http.Handler) http.Handler {
		return authMiddleware(client, h)
	}
	viewer := func(h http.Handler) http.Handler {
		return authMiddleware(client, requireRole("viewer", h))
	}
	creator := func(h http.Handler) http.Handler {
		return authMiddleware(client, requireRole("creator", h))
	}
	admin := func(h http.Handler) http.Handler {
		return authMiddleware(client, requireRole("admin", h))
	}

	// ── Public ────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /auth/login", handleAuthLogin)
	mux.HandleFunc("POST /auth/token", handleAuthToken)
	mux.HandleFunc("POST /auth/refresh", handleAuthRefresh)
	mux.HandleFunc("GET /health", handleHealth)

	// ── Me ────────────────────────────────────────────────────────────────────
	mux.Handle("GET /api/me", viewer(http.HandlerFunc(handleMe)))

	// ── Videos ────────────────────────────────────────────────────────────────
	// Reads: viewer+ (any registered user can watch)
	mux.Handle("GET /api/videos", viewer(http.HandlerFunc(handleListVideos)))
	mux.Handle("GET /api/videos/{id}", viewer(http.HandlerFunc(handleGetVideo)))
	// Writes: creator+ (resource-level canAccess check inside handler)
	mux.Handle("POST /api/videos", creator(http.HandlerFunc(handleUploadVideo)))
	mux.Handle("PUT /api/videos/{id}", creator(http.HandlerFunc(handleUpdateVideo)))
	mux.Handle("DELETE /api/videos/{id}", creator(http.HandlerFunc(handleDeleteVideo)))

	// ── Channels ──────────────────────────────────────────────────────────────
	// All operations: creator+ (resource-level canAccess check inside handler)
	mux.Handle("GET /api/channels", creator(http.HandlerFunc(handleListChannels)))
	mux.Handle("GET /api/channels/{id}", creator(http.HandlerFunc(handleGetChannel)))
	mux.Handle("POST /api/channels", creator(http.HandlerFunc(handleCreateChannel)))
	mux.Handle("PUT /api/channels/{id}", creator(http.HandlerFunc(handleUpdateChannel)))
	mux.Handle("DELETE /api/channels/{id}", creator(http.HandlerFunc(handleDeleteChannel)))

	// ── Admin ─────────────────────────────────────────────────────────────────
	mux.Handle("GET /api/admin/users", admin(http.HandlerFunc(handleAdminUsers)))
	mux.Handle("DELETE /api/admin/users/{id}", admin(http.HandlerFunc(handleDeleteAdminUser)))

	// ── Debug (remove in production) ──────────────────────────────────────────
	mux.Handle("GET /api/debug/perms", auth(http.HandlerFunc(handleDebugPerms)))
	mux.Handle("POST /api/debug/grant", auth(http.HandlerFunc(handleDebugGrant)))
	mux.Handle("DELETE /api/debug/revoke", auth(http.HandlerFunc(handleDebugRevoke)))
}
