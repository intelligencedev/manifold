package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	oauth2StateCookie       = "oauth2_state"
	oauth2VerifierCookie    = "oauth2_code_verifier"
	defaultRedirectFallback = "/auth/login"
)

// OAuth2Options describe how to talk to a non-OIDC OAuth2 provider.
type OAuth2Options struct {
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	AuthURL             string
	TokenURL            string
	UserInfoURL         string
	LogoutURL           string
	LogoutRedirectParam string
	Scopes              []string
	ProviderName        string
	DefaultRoles        []string
	EmailField          string
	NameField           string
	PictureField        string
	SubjectField        string
	RolesField          string
	CookieName          string
	AllowedDomains      []string
	StateTTLSeconds     int
	TempCookieSecure    bool
	HTTPClient          *http.Client
}

// OAuth2 implements a plain OAuth2 Authorization Code + PKCE login handler.
type OAuth2 struct {
	oauth2Config        *oauth2.Config
	store               *Store
	cookieName          string
	allowedDomains      []string
	stateTTL            time.Duration
	tempCookieSecure    bool
	providerName        string
	userInfoURL         string
	logoutURL           string
	logoutRedirectParam string
	emailField          string
	nameField           string
	pictureField        string
	subjectField        string
	rolesField          string
	defaultRoles        []string
	httpClient          *http.Client
}

// NewOAuth2 constructs a new OAuth2 provider backed by the given options.
func NewOAuth2(ctx context.Context, store *Store, opts OAuth2Options) (*OAuth2, error) {
	if store == nil {
		return nil, errors.New("store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	authURL := strings.TrimSpace(opts.AuthURL)
	tokenURL := strings.TrimSpace(opts.TokenURL)
	userInfoURL := strings.TrimSpace(opts.UserInfoURL)
	if authURL == "" || tokenURL == "" || userInfoURL == "" {
		return nil, errors.New("authURL, tokenURL, and userInfoURL are required for oauth2 provider")
	}
	scopes := opts.Scopes
	if len(scopes) == 0 {
		scopes = []string{"profile", "email"}
	}
	conf := &oauth2.Config{
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		RedirectURL:  opts.RedirectURL,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
	}
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
		providerName = "oauth2"
	}
	logoutParam := strings.TrimSpace(opts.LogoutRedirectParam)
	if logoutParam == "" {
		logoutParam = "post_logout_redirect_uri"
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		if c, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok && c != nil {
			httpClient = c
		}
	}
	return &OAuth2{
		oauth2Config:        conf,
		store:               store,
		cookieName:          cookieName,
		allowedDomains:      opts.AllowedDomains,
		stateTTL:            ttl,
		tempCookieSecure:    opts.TempCookieSecure,
		providerName:        providerName,
		userInfoURL:         userInfoURL,
		logoutURL:           strings.TrimSpace(opts.LogoutURL),
		logoutRedirectParam: logoutParam,
		emailField:          fieldOrDefault(opts.EmailField, "email"),
		nameField:           fieldOrDefault(opts.NameField, "name"),
		pictureField:        strings.TrimSpace(opts.PictureField),
		subjectField:        strings.TrimSpace(opts.SubjectField),
		rolesField:          strings.TrimSpace(opts.RolesField),
		defaultRoles:        normalizeDefaultRoles(opts.DefaultRoles),
		httpClient:          httpClient,
	}, nil
}

// LoginHandler begins the OAuth2 authorization code flow with PKCE.
func (o *OAuth2) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, _ := randToken(16)
		cv, _ := randToken(32)
		challenge := pkceChallenge(cv)
		https := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		secure := o.tempCookieSecure && https
		setTempCookie(w, oauth2StateCookie, state, o.stateTTL, secure)
		setTempCookie(w, oauth2VerifierCookie, cv, o.stateTTL, secure)
		url := o.oauth2Config.AuthCodeURL(
			state,
			oauth2.SetAuthURLParam("code_challenge", challenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)
		http.Redirect(w, r, url, http.StatusFound)
	}
}

// CallbackHandler completes the OAuth2 authorization, creates user and session, and sets cookie.
func (o *OAuth2) CallbackHandler(cookieSecure bool, cookieDomain string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		if state == "" || code == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		sc, err := r.Cookie(oauth2StateCookie)
		if err != nil || sc.Value != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		vc, err := r.Cookie(oauth2VerifierCookie)
		if err != nil || vc.Value == "" {
			http.Error(w, "missing code verifier", http.StatusBadRequest)
			return
		}
		ctx := o.withHTTPClient(r.Context())
		tok, err := o.oauth2Config.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", vc.Value))
		if err != nil {
			http.Error(w, "exchange failed", http.StatusBadRequest)
			return
		}
		payload, err := o.fetchUserInfo(ctx, tok)
		if err != nil {
			http.Error(w, "userinfo failed", http.StatusBadGateway)
			return
		}
		email := o.stringField(payload, o.emailField)
		if email == "" {
			http.Error(w, "email required", http.StatusForbidden)
			return
		}
		if !EmailAllowed(email, o.allowedDomains) {
			http.Error(w, "email domain not allowed", http.StatusForbidden)
			return
		}
		name := o.stringField(payload, o.nameField)
		if name == "" {
			name = email
		}
		picture := o.stringField(payload, o.pictureField)
		subject := o.stringField(payload, o.subjectField)
		if subject == "" {
			subject = email
		}
		u := &User{Email: email, Name: name, Picture: picture, Provider: o.providerName, Subject: subject}
		u, err = o.store.UpsertUser(ctx, u)
		if err != nil {
			http.Error(w, "user upsert", http.StatusInternalServerError)
			return
		}
		roles := o.rolesFromPayload(payload)
		if err := o.store.SetUserRoles(ctx, u.ID, roles); err != nil {
			http.Error(w, "role sync", http.StatusInternalServerError)
			return
		}
		sess, err := o.store.CreateSession(ctx, u.ID)
		if err != nil {
			http.Error(w, "session create", http.StatusInternalServerError)
			return
		}
		cookie := &http.Cookie{
			Name:     o.cookieName,
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
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// LogoutHandler deletes the session and clears the cookie, optionally redirecting through the IdP.
func (o *OAuth2) LogoutHandler(cookieSecure bool, cookieDomain string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(o.cookieName); err == nil && c.Value != "" {
			_ = o.store.DeleteSession(r.Context(), c.Value)
		}
		cookie := &http.Cookie{
			Name:     o.cookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   cookieSecure,
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
		}
		if cookieDomain != "" {
			cookie.Domain = cookieDomain
		}
		http.SetCookie(w, cookie)
		next := absoluteRedirectURL(r, r.URL.Query().Get("next"), defaultRedirectFallback)
		if o.logoutURL == "" || next == "" {
			http.Redirect(w, r, next, http.StatusFound)
			return
		}
		http.Redirect(w, r, appendLogoutRedirect(o.logoutURL, o.logoutRedirectParam, next), http.StatusFound)
	}
}

// MeHandler returns basic info about the current user.
func (o *OAuth2) MeHandler() http.HandlerFunc {
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

func (o *OAuth2) fetchUserInfo(ctx context.Context, tok *oauth2.Token) (map[string]any, error) {
	ctx = o.withHTTPClient(ctx)
	client := o.oauth2Config.Client(ctx, tok)
	resp, err := client.Get(o.userInfoURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("userinfo status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (o *OAuth2) rolesFromPayload(payload map[string]any) []string {
	roleSet := map[string]struct{}{}
	for _, r := range o.defaultRoles {
		roleSet[r] = struct{}{}
	}
	for _, r := range extractRoles(payload, o.rolesField) {
		roleSet[r] = struct{}{}
	}
	out := make([]string, 0, len(roleSet))
	for role := range roleSet {
		out = append(out, role)
	}
	sort.Strings(out)
	return out
}

func (o *OAuth2) stringField(payload map[string]any, field string) string {
	if strings.TrimSpace(field) == "" {
		return ""
	}
	val, ok := dig(payload, field)
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	}
	return ""
}

func (o *OAuth2) withHTTPClient(ctx context.Context) context.Context {
	if o.httpClient == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, o.httpClient)
}

func fieldOrDefault(value, def string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return def
	}
	return v
}

func normalizeDefaultRoles(in []string) []string {
	set := map[string]struct{}{"user": {}}
	for _, r := range in {
		n := normalizeRoleName(r)
		if n == "" {
			continue
		}
		set[n] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for role := range set {
		out = append(out, role)
	}
	sort.Strings(out)
	return out
}

func extractRoles(payload map[string]any, path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	val, ok := dig(payload, path)
	if !ok || val == nil {
		return nil
	}
	result := map[string]struct{}{}
	switch v := val.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				n := normalizeRoleName(s)
				if n != "" {
					result[n] = struct{}{}
				}
			}
		}
	case []string:
		for _, s := range v {
			n := normalizeRoleName(s)
			if n != "" {
				result[n] = struct{}{}
			}
		}
	case string:
		for _, part := range strings.Split(v, ",") {
			n := normalizeRoleName(part)
			if n != "" {
				result[n] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(result))
	for role := range result {
		out = append(out, role)
	}
	sort.Strings(out)
	return out
}

func dig(payload map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	cur := any(payload)
	parts := strings.Split(path, ".")
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		val, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = val
	}
	return cur, true
}

func appendLogoutRedirect(base, param, redirect string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	key := param
	if strings.TrimSpace(key) == "" {
		key = "post_logout_redirect_uri"
	}
	q.Set(key, redirect)
	u.RawQuery = q.Encode()
	return u.String()
}
