package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

// =============================================================================
// Logging middleware
// =============================================================================

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.WriteHeader(http.StatusNoContent)
			log.Printf("OPTIONS %s 204", r.URL.Path)
			return
		}

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d", r.Method, r.URL.Path, rw.status)
	})
}

// =============================================================================
// Auth middleware
// =============================================================================

// authMiddleware validates the Bearer token by verifying its JWT signature
// against Zitadel's JWKS endpoint (keys are cached locally).
// On success it stores a userContext in the request context.
func authMiddleware(client *http.Client, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken, err := extractBearerToken(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}

		validateCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		claims, err := validateJWT(validateCtx, client, rawToken)
		if err != nil {
			log.Printf("JWT validation error: %v", err)
			writeError(w, http.StatusUnauthorized, "token validation failed")
			return
		}

		if !audienceMatches([]string(claims.Audience), cfg.requiredAudience) {
			writeError(w, http.StatusForbidden, "token audience mismatch")
			return
		}

		uc := userContext{
			Sub:      claims.Subject,
			Name:     claims.Name,
			Email:    claims.Email,
			Username: claims.Username,
			Scopes:   parseScopes(claims.Scope),
			Roles:    extractRoleNames(claims.Roles),
			OrgID:    claims.OrgID,
			OrgName:  claims.OrgName,
		}

		ctx := context.WithValue(r.Context(), userContextKey, uc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// =============================================================================
// Authorization middleware
// =============================================================================

// platformRank defines the hierarchy for platform-level JWT roles.
// A higher-ranked role satisfies all lower-ranked route requirements.
var platformRank = map[string]int{
	"viewer":  1,
	"creator": 2,
	"admin":   3,
}

// requireRole gates a route by minimum platform role from the JWT.
// Hierarchy applies: admin passes creator routes, creator passes viewer routes.
// Resource-level roles (viewer/creator/admin in permStore) are separate —
// those are checked inside handlers via canAccess().
func requireRole(minRole string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := getUserFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		for _, role := range user.Roles {
			if platformRank[role] >= platformRank[minRole] {
				next.ServeHTTP(w, r)
				return
			}
		}
		writeError(w, http.StatusForbidden, fmt.Sprintf("platform role %q or higher required", minRole))
	})
}
