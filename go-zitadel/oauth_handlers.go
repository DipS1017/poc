package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// =============================================================================
// Helpers
// =============================================================================

// jwtSub extracts the sub claim from a JWT without full verification.
// Used only to seed the mock permission store after a successful token exchange.
func jwtSub(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Sub
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// =============================================================================
// Zitadel token response
// =============================================================================

type zitadelTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func exchangeCode(ctx context.Context, code string) (zitadelTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", cfg.redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.zitadelDomain+"/oauth/v2/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return zitadelTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(cfg.oauthClientID, cfg.oauthClientSecret)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return zitadelTokenResponse{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return zitadelTokenResponse{}, fmt.Errorf("zitadel returned %d: %s", resp.StatusCode, body)
	}

	var result zitadelTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return zitadelTokenResponse{}, err
	}
	return result, nil
}

// =============================================================================
// Handlers
// =============================================================================

func handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", cfg.oauthClientID)
	params.Set("redirect_uri", cfg.redirectURI)
	params.Set("scope", "openid profile email offline_access urn:zitadel:iam:org:projects:roles")
	params.Set("state", state)

	http.Redirect(w, r, cfg.zitadelDomain+"/oauth/v2/authorize?"+params.Encode(), http.StatusFound)
}

type tokenRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

func handleAuthToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cookie, err := r.Cookie("oauth_state")
	if err != nil || cookie.Value != req.State {
		writeError(w, http.StatusBadRequest, "state mismatch — possible CSRF")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		MaxAge: -1,
		Path:   "/",
	})

	tokens, err := exchangeCode(r.Context(), req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("token exchange failed: %v", err))
		return
	}

	// Seed mock permissions for this user so the mock data is accessible.
	if sub := jwtSub(tokens.AccessToken); sub != "" {
		seedPermissions(sub)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"id_token":      tokens.IDToken,
		"expires_in":    tokens.ExpiresIn,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", req.RefreshToken)

	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		cfg.zitadelDomain+"/oauth/v2/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build refresh request")
		return
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.SetBasicAuth(cfg.oauthClientID, cfg.oauthClientSecret)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}
	defer resp.Body.Close()

	var tokens zitadelTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse refresh response")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_in":    tokens.ExpiresIn,
	})
}
