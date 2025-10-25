package auth

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDC struct {
	Provider         *oidc.Provider
	OAuth2Config     *oauth2.Config
	Verifier         *oidc.IDTokenVerifier
	Store            *Store
	CookieName       string
	AllowedDomains   []string
	StateTTL         time.Duration
	TempCookieSecure bool
	// Issuer base URL (e.g., https://keycloak.example/realms/myrealm)
	Issuer string
}

type Claims struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	// 'sub' provided by oidc.Verifier extraction
	RealmAccess struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	ResourceAccess map[string]struct {
		Roles []string `json:"roles"`
	} `json:"resource_access"`
	Groups []string `json:"groups"`
}

func NewOIDC(ctx context.Context, issuer, clientID, clientSecret, redirectURL string, store *Store, cookieName string, allowedDomains []string, stateTTLSeconds int, tempCookieSecure bool) (*OIDC, error) {
	prov, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     prov.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
	v := prov.Verifier(&oidc.Config{ClientID: clientID})
	if cookieName == "" {
		cookieName = "sio_session"
	}
	ttl := time.Duration(stateTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &OIDC{Provider: prov, OAuth2Config: conf, Verifier: v, Store: store, CookieName: cookieName, AllowedDomains: allowedDomains, StateTTL: ttl, TempCookieSecure: tempCookieSecure, Issuer: issuer}, nil
}

// LoginHandler begins the OIDC authorization code flow with PKCE.
func (o *OIDC) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Create state and PKCE code verifier
		state, _ := randToken(16)
		cv, _ := randToken(32)
		cChallenge := pkceChallenge(cv)
		// Save state+cv to short-lived cookies. Honor HTTPS at runtime even if config says secure.
		// If request is HTTP (no TLS and not forwarded as https), do not mark Secure to ensure browser sends it back.
		https := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		secure := o.TempCookieSecure && https
		setTempCookie(w, "oidc_state", state, o.StateTTL, secure)
		setTempCookie(w, "oidc_code_verifier", cv, o.StateTTL, secure)
		// Build AuthCodeURL with PKCE
		url := o.OAuth2Config.AuthCodeURL(state, oauth2.SetAuthURLParam("code_challenge", cChallenge), oauth2.SetAuthURLParam("code_challenge_method", "S256"))
		http.Redirect(w, r, url, http.StatusFound)
	}
}

// CallbackHandler completes the OIDC authorization, creates user and session, and sets cookie.
func (o *OIDC) CallbackHandler(cookieSecure bool, cookieDomain string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate state
		st := r.URL.Query().Get("state")
		cc := r.URL.Query().Get("code")
		if st == "" || cc == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		sc, err := r.Cookie("oidc_state")
		if err != nil || sc.Value != st {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		cvc, err := r.Cookie("oidc_code_verifier")
		if err != nil || cvc.Value == "" {
			http.Error(w, "missing code verifier", http.StatusBadRequest)
			return
		}
		// Exchange code with PKCE
		ctx := r.Context()
		tok, err := o.OAuth2Config.Exchange(ctx, cc, oauth2.SetAuthURLParam("code_verifier", cvc.Value))
		if err != nil {
			http.Error(w, "exchange failed", http.StatusBadRequest)
			return
		}
		rawID, ok := tok.Extra("id_token").(string)
		if !ok {
			http.Error(w, "missing id_token", http.StatusBadRequest)
			return
		}
		idt, err := o.Verifier.Verify(ctx, rawID)
		if err != nil {
			http.Error(w, "verify failed", http.StatusUnauthorized)
			return
		}
		var c Claims
		if err := idt.Claims(&c); err != nil {
			http.Error(w, "claims decode", http.StatusBadRequest)
			return
		}
		if c.Email == "" {
			http.Error(w, "email required", http.StatusForbidden)
			return
		}
		if !EmailAllowed(c.Email, o.AllowedDomains) {
			http.Error(w, "email domain not allowed", http.StatusForbidden)
			return
		}
		u := &User{Email: c.Email, Name: c.Name, Picture: c.Picture, Provider: "oidc", Subject: idt.Subject}
		u, err = o.Store.UpsertUser(ctx, u)
		if err != nil {
			http.Error(w, "user upsert", http.StatusInternalServerError)
			return
		}
		roles := rolesFromClaims(c)
		if len(roles) == 0 {
			roles = []string{"user"}
		}
		if err := o.Store.SetUserRoles(ctx, u.ID, roles); err != nil {
			http.Error(w, "role sync", http.StatusInternalServerError)
			return
		}
		sess, err := o.Store.CreateSession(ctx, u.ID)
		if err != nil {
			http.Error(w, "session create", http.StatusInternalServerError)
			return
		}
		// Set secure, httpOnly cookie
		cookie := &http.Cookie{
			Name:     o.CookieName,
			Value:    sess.ID,
			Path:     "/",
			HttpOnly: true,
			Secure:   cookieSecure,
			SameSite: http.SameSiteLaxMode,
		}
		if cookieDomain != "" {
			cookie.Domain = cookieDomain
		}
		http.SetCookie(w, cookie)
		// Persist ID token server-side for RP-initiated logout
		_ = o.Store.SetSessionIDToken(ctx, sess.ID, rawID)
		// Redirect to UI root
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// LogoutHandler deletes the session and clears the cookie.
func (o *OIDC) LogoutHandler(cookieSecure bool, cookieDomain string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Load session cookie and capture id_token (if any) before deleting session
		var idToken string
		c, err := r.Cookie(o.CookieName)
		if err == nil && c != nil && c.Value != "" {
			if sess, _, err := o.Store.GetSession(r.Context(), c.Value); err == nil && sess != nil {
				idToken = sess.IDToken
			}
			// Delete the session
			_ = o.Store.DeleteSession(r.Context(), c.Value)
		}
		// Clear cookie
		http.SetCookie(w, &http.Cookie{
			Name:     o.CookieName,
			Value:    "",
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   cookieSecure,
			SameSite: http.SameSiteLaxMode,
			Domain:   cookieDomain,
		})
		// No id_token cookie used anymore
		// Determine where the app should land after IdP logout
		next := r.URL.Query().Get("next")
		absNext := absoluteRedirectURL(r, next, "/auth/login")
		// For Keycloak (and many OIDC providers), perform RP-initiated logout to end SSO session
		// Keycloak end-session endpoint: {issuer}/protocol/openid-connect/logout
		// Use client_id and post_logout_redirect_uri. id_token_hint is optional for browser-initiated logout.
		logoutBase := strings.TrimSuffix(o.Issuer, "/") + "/protocol/openid-connect/logout"
		q := url.Values{}
		q.Set("client_id", o.OAuth2Config.ClientID)
		q.Set("post_logout_redirect_uri", absNext)
		if idToken != "" {
			q.Set("id_token_hint", idToken)
		}
		kcLogout := logoutBase + "?" + q.Encode()
		http.Redirect(w, r, kcLogout, http.StatusFound)
	}
}

func rolesFromClaims(c Claims) []string {
	roles := map[string]struct{}{"user": {}}
	if claimsContain(c, "admin") {
		roles["admin"] = struct{}{}
	}
	out := make([]string, 0, len(roles))
	for role := range roles {
		out = append(out, role)
	}
	sort.Strings(out)
	return out
}

func normalizeRoleName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.TrimPrefix(name, "/")
	return strings.ToLower(name)
}

func claimsContain(c Claims, want string) bool {
	w := normalizeRoleName(want)
	for _, role := range c.RealmAccess.Roles {
		if normalizeRoleName(role) == w {
			return true
		}
	}
	for _, entry := range c.ResourceAccess {
		for _, role := range entry.Roles {
			if normalizeRoleName(role) == w {
				return true
			}
		}
	}
	for _, g := range c.Groups {
		if normalizeRoleName(g) == w {
			return true
		}
	}
	return false
}

// MeHandler returns basic info about the current user.
func (o *OIDC) MeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if u, ok := CurrentUser(r.Context()); ok && u != nil {
			_, _ = w.Write([]byte(`{"email":"` + u.Email + `","name":"` + u.Name + `","picture":"` + u.Picture + `"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}
}
