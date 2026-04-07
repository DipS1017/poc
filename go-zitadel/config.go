package main

// authConfig holds all settings for JWT validation and the OAuth2 login flow.
type authConfig struct {
	// shared
	zitadelDomain string

	// JWKS-based local JWT validation
	jwksURL          string
	requiredAudience string

	// Web application — used for the login / register flow
	oauthClientID     string
	oauthClientSecret string
	redirectURI       string
	frontendURL       string

}

// cfg is the hardcoded configuration.
// After running `docker compose up`:
//   - Create a Web application → paste oauthClientID + oauthClientSecret below
var cfg = authConfig{
	zitadelDomain: "http://localhost:8080",

	// JWKS endpoint (public keys, no credentials needed)
	jwksURL:         "http://localhost:8080/oauth/v2/keys",
	requiredAudience: "",

	// Web app (login flow)
	oauthClientID:     "360896690300977155",
	oauthClientSecret: "a8eja1Bpk1MeNPUWd34VVfW8QwKTNNorgbox0cbuU2CGi7qaaANrpWbgBmVzaPgf",
	redirectURI: "http://localhost:5173/auth/callback",
	frontendURL: "http://localhost:5173",
}
