package main

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// =============================================================================
// Zitadel types
// =============================================================================

// zitadelRoles maps role name -> (orgID -> orgName) as returned by Zitadel.
type zitadelRoles map[string]map[string]string

// jwtClaims holds the standard JWT claims plus Zitadel-specific fields.
type jwtClaims struct {
	jwt.RegisteredClaims
	Name     string       `json:"name"`
	Email    string       `json:"email"`
	Username string       `json:"preferred_username"`
	Scope    string       `json:"scope"`
	Roles    zitadelRoles `json:"urn:zitadel:iam:org:project:roles"`
	OrgID    string       `json:"urn:zitadel:iam:user:resourceowner:id"`
	OrgName  string       `json:"urn:zitadel:iam:user:resourceowner:name"`
}

// userContext is the application-level view of the authenticated principal
// stored in the request context by authMiddleware.
type userContext struct {
	Sub      string
	Name     string
	Email    string
	Username string
	Scopes   []string
	Roles    []string
	OrgID    string
	OrgName  string
}

// =============================================================================
// Context key
// =============================================================================

type contextKey string

const userContextKey contextKey = "user"

func getUserFromContext(r *http.Request) (userContext, bool) {
	u, ok := r.Context().Value(userContextKey).(userContext)
	return u, ok
}

// =============================================================================
// JWKS cache
// =============================================================================

type jwksCache struct {
	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey // kid → public key
	fetchedAt time.Time
	ttl       time.Duration
}

var keyCache = &jwksCache{
	keys: make(map[string]*rsa.PublicKey),
	ttl:  1 * time.Hour,
}

type jwkKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

// getKey returns the RSA public key for the given kid, refreshing the cache if
// the kid is unknown or the cache has expired.
func (c *jwksCache) getKey(ctx context.Context, client *http.Client, kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	expired := time.Since(c.fetchedAt) > c.ttl
	c.mu.RUnlock()

	if ok && !expired {
		return key, nil
	}

	if err := c.refresh(ctx, client); err != nil {
		return nil, fmt.Errorf("refresh JWKS: %w", err)
	}

	c.mu.RLock()
	key, ok = c.keys[kid]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("kid %q not found in JWKS", kid)
	}
	return key, nil
}

func (c *jwksCache) refresh(ctx context.Context, client *http.Client) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.jwksURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.Unmarshal(body, &jwks); err != nil {
		return err
	}

	newKeys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.Use != "sig" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		newKeys[k.Kid] = pub
	}

	c.keys = newKeys
	c.fetchedAt = time.Now()
	return nil
}

func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// =============================================================================
// JWT validation
// =============================================================================

func validateJWT(ctx context.Context, client *http.Client, rawToken string) (jwtClaims, error) {
	var claims jwtClaims
	token, err := jwt.ParseWithClaims(rawToken, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("missing kid in token header")
		}
		return keyCache.getKey(ctx, client, kid)
	})
	if err != nil {
		return jwtClaims{}, err
	}
	if !token.Valid {
		return jwtClaims{}, errors.New("token is not valid")
	}
	return claims, nil
}

// =============================================================================
// Token helpers
// =============================================================================

func extractBearerToken(r *http.Request) (string, error) {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return "", errors.New("missing Authorization header")
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("Authorization header must be 'Bearer <token>'")
	}
	return strings.TrimSpace(parts[1]), nil
}

// =============================================================================
// Auth helpers
// =============================================================================

func parseScopes(scopeStr string) []string {
	return strings.Fields(scopeStr)
}

func extractRoleNames(roles zitadelRoles) []string {
	if roles == nil {
		return nil
	}
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	return names
}

func hasRole(roles []string, required string) bool {
	for _, r := range roles {
		if r == required {
			return true
		}
	}
	return false
}

func audienceMatches(tokenAud []string, requiredAud string) bool {
	if requiredAud == "" {
		return true
	}
	for _, a := range tokenAud {
		if a == requiredAud {
			return true
		}
	}
	return false
}
