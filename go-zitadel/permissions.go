package main

import "strings"

// Resource-level roles (never in JWT — permStore only):
//   viewer  — can read/watch
//   creator — made the resource, can edit and delete it
//   admin   — full control over any resource
var roleRank = map[string]int{
	"viewer":  1,
	"creator": 2,
	"admin":   3,
}

// =============================================================================
// permStore — source of truth (swap for Postgres in production)
// =============================================================================

var permStore = map[string]string{} // "userID|resourceType|resourceID" → role

func permKey(userID, resourceType, resourceID string) string {
	return userID + "|" + resourceType + "|" + resourceID
}

// =============================================================================
// permCache — per-user snapshot loaded at login
//
// Flow:
//   login → loadUserPermissions(sub)  builds cache from permStore
//   request → canAccess(...)          reads from cache, zero DB hit
//   grant/revoke                      writes to both permStore AND cache
// =============================================================================

// userPerms is a flat map of "resourceType|resourceID" → role for one user.
type userPerms map[string]string

var permCache = map[string]userPerms{} // userID → their permissions

// loadUserPermissions scans permStore for the given user and builds their cache.
// Call this once at login. In production, replace the permStore scan with
// a single DB query: SELECT resource_type, resource_id, role WHERE user_id=$1
func loadUserPermissions(userID string) {
	snapshot := userPerms{}
	for k, role := range permStore {
		parts := strings.SplitN(k, "|", 3)
		if len(parts) == 3 && parts[0] == userID {
			snapshot[parts[1]+"|"+parts[2]] = role
		}
	}
	permCache[userID] = snapshot
}

// canAccess returns true if userID holds at least minRole on the resource.
// Reads from the per-user cache — no DB hit.
// Falls back to permStore directly if the user hasn't logged in yet.
func canAccess(userID, resourceType, resourceID, minRole string) bool {
	if cache, ok := permCache[userID]; ok {
		role, ok := cache[resourceType+"|"+resourceID]
		return ok && roleRank[role] >= roleRank[minRole]
	}
	// fallback for users without a cache entry
	role, ok := permStore[permKey(userID, resourceType, resourceID)]
	return ok && roleRank[role] >= roleRank[minRole]
}

// grantAccess writes to permStore and updates the cache immediately.
func grantAccess(userID, resourceType, resourceID, role string) {
	permStore[permKey(userID, resourceType, resourceID)] = role
	if cache, ok := permCache[userID]; ok {
		cache[resourceType+"|"+resourceID] = role
	}
}

// revokeAccess removes from permStore and the cache immediately.
func revokeAccess(userID, resourceType, resourceID string) {
	delete(permStore, permKey(userID, resourceType, resourceID))
	if cache, ok := permCache[userID]; ok {
		delete(cache, resourceType+"|"+resourceID)
	}
}

// seedPermissions pre-loads mock resource ownership and builds the cache.
// In production this is replaced by loadUserPermissions after a real DB query.
func seedPermissions(userID string) {
	for _, id := range []string{"v1", "v2"} {
		permStore[permKey(userID, "video", id)] = "creator"
	}
	permStore[permKey(userID, "channel", "c1")] = "creator"
	loadUserPermissions(userID)
}
