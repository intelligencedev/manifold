package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDC struct {
	Provider            *oidc.Provider
	OAuth2Config        *oauth2.Config
	Verifier            *oidc.IDTokenVerifier
	Store               *Store
	CookieName          string
	AllowedDomains      []string
	StateTTL            time.Duration
	TempCookieSecure    bool
	ResponseMode        string
	ProviderName        string
	LogoutURL           string
	LogoutRedirectParam string
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

type OIDCOptions struct {
	IssuerURL           string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	Store               *Store
	CookieName          string
	AllowedDomains      []string
	StateTTLSeconds     int
	TempCookieSecure    bool
	Scopes              []string
	ResponseMode        string
	TokenAuthStyle      oauth2.AuthStyle
	ProviderName        string
	LogoutURL           string
	LogoutRedirectParam string
}

func NewOIDC(ctx context.Context, opts OIDCOptions) (*OIDC, error) {
	prov, err := oidc.NewProvider(ctx, opts.IssuerURL)
	if err != nil {
		return nil, err
	}
	conf := &oauth2.Config{
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		Endpoint:     prov.Endpoint(),
		RedirectURL:  opts.RedirectURL,
		Scopes:       oidcScopes(opts.Scopes),
	}
	if opts.TokenAuthStyle != 0 {
		conf.Endpoint.AuthStyle = opts.TokenAuthStyle
	}
	v := prov.Verifier(&oidc.Config{ClientID: opts.ClientID})
	cookieName := opts.CookieName
	if cookieName == "" {
		cookieName = "sio_session"
	}
	ttl := time.Duration(opts.StateTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	providerName := strings.TrimSpace(opts.ProviderName)
	if providerName == "" {
		providerName = "oidc"
	}
	return &OIDC{
		Provider:            prov,
		OAuth2Config:        conf,
		Verifier:            v,
		Store:               opts.Store,
		CookieName:          cookieName,
		AllowedDomains:      opts.AllowedDomains,
		StateTTL:            ttl,
		TempCookieSecure:    opts.TempCookieSecure,
		ResponseMode:        strings.TrimSpace(opts.ResponseMode),
		ProviderName:        providerName,
		LogoutURL:           strings.TrimSpace(opts.LogoutURL),
		LogoutRedirectParam: strings.TrimSpace(opts.LogoutRedirectParam),
		Issuer:              opts.IssuerURL,
	}, nil
}

// LoginHandler begins the OIDC authorization code flow with PKCE.
func (o *OIDC) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := randToken(16)
		if err != nil {
			http.Error(w, "failed to initialize login", http.StatusInternalServerError)
			return
		}
		cv, err := randToken(32)
		if err != nil {
			http.Error(w, "failed to initialize login", http.StatusInternalServerError)
			return
		}
		cChallenge := pkceChallenge(cv)
		// Save state+cv to short-lived cookies. Honor HTTPS at runtime even if config says secure.
		// If request is HTTP (no TLS and not forwarded as https), do not mark Secure to ensure browser sends it back.
		https := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		secure := o.TempCookieSecure && https
		setTempCookie(w, "oidc_state", state, o.StateTTL, secure)
		setTempCookie(w, "oidc_code_verifier", cv, o.StateTTL, secure)
		params := []oauth2.AuthCodeOption{
			oauth2.SetAuthURLParam("code_challenge", cChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		}
		if o.ResponseMode != "" {
			params = append(params, oauth2.SetAuthURLParam("response_mode", o.ResponseMode))
		}
		url := o.OAuth2Config.AuthCodeURL(state, params...)
		http.Redirect(w, r, url, http.StatusFound)
	}
}

// CallbackHandler completes the OIDC authorization, creates user and session, and sets cookie.
func (o *OIDC) CallbackHandler(cookieSecure bool, cookieDomain string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o.handleCallback(w, r, cookieSecure, cookieDomain)
	}
}

func (o *OIDC) handleCallback(w http.ResponseWriter, r *http.Request, cookieSecure bool, cookieDomain string) {
	req, ok := o.readCallbackRequest(w, r)
	if !ok {
		return
	}
	rawID, idt, ok := o.verifyCallbackToken(w, r, req)
	if !ok {
		return
	}
	u, ok := o.syncCallbackUser(w, r, idt, req.AppleUser)
	if !ok {
		return
	}
	sess, err := o.Store.CreateSession(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "session create", http.StatusInternalServerError)
		return
	}
	o.setSessionCookie(w, sess.ID, cookieSecure, cookieDomain)
	_ = o.Store.SetSessionIDToken(r.Context(), sess.ID, rawID)
	http.Redirect(w, r, "/", http.StatusFound)
}

type oidcCallbackRequest struct {
	Code         string
	CodeVerifier string
	AppleUser    string
}

func (o *OIDC) readCallbackRequest(w http.ResponseWriter, r *http.Request) (oidcCallbackRequest, bool) {
	if err := parseOIDCCallbackForm(r); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return oidcCallbackRequest{}, false
	}
	state, code := r.Form.Get("state"), r.Form.Get("code")
	if state == "" || code == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return oidcCallbackRequest{}, false
	}
	if sc, err := r.Cookie("oidc_state"); err != nil || sc.Value != state {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return oidcCallbackRequest{}, false
	}
	verifier, err := r.Cookie("oidc_code_verifier")
	if err != nil || verifier.Value == "" {
		http.Error(w, "missing code verifier", http.StatusBadRequest)
		return oidcCallbackRequest{}, false
	}
	return oidcCallbackRequest{Code: code, CodeVerifier: verifier.Value, AppleUser: r.Form.Get("user")}, true
}

func (o *OIDC) verifyCallbackToken(w http.ResponseWriter, r *http.Request, req oidcCallbackRequest) (string, *oidc.IDToken, bool) {
	tok, err := o.OAuth2Config.Exchange(r.Context(), req.Code, oauth2.SetAuthURLParam("code_verifier", req.CodeVerifier))
	if err != nil {
		http.Error(w, "exchange failed", http.StatusBadRequest)
		return "", nil, false
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		http.Error(w, "missing id_token", http.StatusBadRequest)
		return "", nil, false
	}
	idt, err := o.Verifier.Verify(r.Context(), rawID)
	if err != nil {
		http.Error(w, "verify failed", http.StatusUnauthorized)
		return "", nil, false
	}
	return rawID, idt, true
}

func (o *OIDC) syncCallbackUser(w http.ResponseWriter, r *http.Request, idt *oidc.IDToken, appleUser string) (*User, bool) {
	var claims Claims
	if err := idt.Claims(&claims); err != nil {
		http.Error(w, "claims decode", http.StatusBadRequest)
		return nil, false
	}
	mergeAppleFormUser(&claims, appleUser)
	if !o.validateCallbackClaims(w, claims) {
		return nil, false
	}
	u := &User{Email: claims.Email, Name: claims.Name, Picture: claims.Picture, Provider: o.ProviderName, Subject: idt.Subject}
	u, err := o.Store.UpsertUser(r.Context(), u)
	if err != nil {
		http.Error(w, "user upsert", http.StatusInternalServerError)
		return nil, false
	}
	if err := o.Store.SetUserRoles(r.Context(), u.ID, callbackRoles(claims)); err != nil {
		http.Error(w, "role sync", http.StatusInternalServerError)
		return nil, false
	}
	return u, true
}

func callbackRoles(claims Claims) []string {
	roles := rolesFromClaims(claims)
	if len(roles) == 0 {
		return []string{"user"}
	}
	return roles
}

func (o *OIDC) validateCallbackClaims(w http.ResponseWriter, claims Claims) bool {
	if claims.Email == "" {
		http.Error(w, "email required", http.StatusForbidden)
		return false
	}
	if !EmailAllowed(claims.Email, o.AllowedDomains) {
		http.Error(w, "email domain not allowed", http.StatusForbidden)
		return false
	}
	return true
}

func (o *OIDC) setSessionCookie(w http.ResponseWriter, sessionID string, secure bool, domain string) {
	cookie := &http.Cookie{
		Name:     o.CookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	if domain != "" {
		cookie.Domain = domain
	}
	http.SetCookie(w, cookie)
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
		logoutBase := o.LogoutURL
		if logoutBase == "" {
			if isKeycloakIssuer(o.Issuer) {
				logoutBase = strings.TrimSuffix(o.Issuer, "/") + "/protocol/openid-connect/logout"
			} else {
				http.Redirect(w, r, absNext, http.StatusFound)
				return
			}
		}
		q := url.Values{}
		q.Set("client_id", o.OAuth2Config.ClientID)
		redirectParam := o.LogoutRedirectParam
		if strings.TrimSpace(redirectParam) == "" {
			redirectParam = "post_logout_redirect_uri"
		}
		q.Set(redirectParam, absNext)
		if idToken != "" {
			q.Set("id_token_hint", idToken)
		}
		logoutURL, err := url.Parse(logoutBase)
		if err != nil {
			http.Redirect(w, r, absNext, http.StatusFound)
			return
		}
		query := logoutURL.Query()
		for k, values := range q {
			for _, value := range values {
				query.Set(k, value)
			}
		}
		logoutURL.RawQuery = query.Encode()
		http.Redirect(w, r, logoutURL.String(), http.StatusFound)
	}
}

func oidcScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{oidc.ScopeOpenID, "email", "profile"}
	}
	hasOpenID := false
	out := make([]string, 0, len(scopes)+1)
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if scope == oidc.ScopeOpenID {
			hasOpenID = true
		}
		out = append(out, scope)
	}
	if !hasOpenID {
		out = append([]string{oidc.ScopeOpenID}, out...)
	}
	return out
}

func parseOIDCCallbackForm(r *http.Request) error {
	switch r.Method {
	case http.MethodGet:
		return r.ParseForm()
	case http.MethodPost:
		return r.ParseForm()
	default:
		return errors.New("unsupported callback method")
	}
}

type appleFormUser struct {
	Email string `json:"email"`
	Name  struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	} `json:"name"`
}

func mergeAppleFormUser(c *Claims, raw string) {
	if c == nil || strings.TrimSpace(raw) == "" {
		return
	}
	var user appleFormUser
	if err := json.Unmarshal([]byte(raw), &user); err != nil {
		return
	}
	if c.Email == "" {
		c.Email = strings.TrimSpace(user.Email)
	}
	if c.Name == "" {
		c.Name = strings.TrimSpace(strings.Join([]string{user.Name.FirstName, user.Name.LastName}, " "))
	}
}

func isKeycloakIssuer(issuer string) bool {
	return strings.Contains(strings.ToLower(issuer), "/realms/")
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
		enc := json.NewEncoder(w)
		if u, ok := CurrentUser(r.Context()); ok && u != nil {
			_ = enc.Encode(map[string]string{
				"email":   u.Email,
				"name":    u.Name,
				"picture": u.Picture,
			})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_ = enc.Encode(map[string]string{"error": "unauthorized"})
	}
}
