package agentd

import (
	"context"
	"fmt"
	"manifold/internal/auth"
	"manifold/internal/persistence/databases"
	"strings"
)

func (a *app) initAuth(ctx context.Context) error {
	if !a.cfg.Auth.Enabled {
		return nil
	}

	dsn := a.cfg.Databases.DefaultDSN
	if dsn == "" {
		return fmt.Errorf("auth enabled but databases.defaultDSN is empty")
	}
	pool, err := databases.OpenPool(ctx, dsn)
	if err != nil {
		return fmt.Errorf("auth db connect failed: %w", err)
	}
	a.authStore = auth.NewStore(pool, a.cfg.Auth.SessionTTLHours)
	if err := a.authStore.InitSchema(ctx); err != nil {
		return fmt.Errorf("auth schema init failed: %w", err)
	}
	_ = a.authStore.EnsureDefaultRoles(ctx)

	providerName := strings.ToLower(strings.TrimSpace(a.cfg.Auth.Provider))
	if providerName == "" {
		providerName = "oidc"
	}
	switch providerName {
	case "oidc":
		if strings.TrimSpace(a.cfg.Auth.IssuerURL) == "" {
			return fmt.Errorf("auth.provider=oidc requires issuerURL")
		}
		if strings.TrimSpace(a.cfg.Auth.ClientID) == "" || strings.TrimSpace(a.cfg.Auth.ClientSecret) == "" {
			return fmt.Errorf("auth.provider=oidc requires clientID and clientSecret")
		}
		oidcAuth, err := auth.NewOIDC(ctx, auth.OIDCOptions{
			IssuerURL:        a.cfg.Auth.IssuerURL,
			ClientID:         a.cfg.Auth.ClientID,
			ClientSecret:     a.cfg.Auth.ClientSecret,
			RedirectURL:      a.cfg.Auth.RedirectURL,
			Store:            a.authStore,
			CookieName:       a.cfg.Auth.CookieName,
			AllowedDomains:   a.cfg.Auth.AllowedDomains,
			StateTTLSeconds:  a.cfg.Auth.StateTTLSeconds,
			TempCookieSecure: a.cfg.Auth.CookieSecure,
		})
		if err != nil {
			return fmt.Errorf("oidc init failed: %w", err)
		}
		a.authProvider = oidcAuth
	case "oauth2":
		opts := auth.OAuth2Options{
			ClientID:            a.cfg.Auth.ClientID,
			ClientSecret:        a.cfg.Auth.ClientSecret,
			RedirectURL:         a.cfg.Auth.RedirectURL,
			AuthURL:             a.cfg.Auth.OAuth2.AuthURL,
			TokenURL:            a.cfg.Auth.OAuth2.TokenURL,
			UserInfoURL:         a.cfg.Auth.OAuth2.UserInfoURL,
			LogoutURL:           a.cfg.Auth.OAuth2.LogoutURL,
			LogoutRedirectParam: a.cfg.Auth.OAuth2.LogoutRedirectParam,
			Scopes:              a.cfg.Auth.OAuth2.Scopes,
			ProviderName:        a.cfg.Auth.OAuth2.ProviderName,
			DefaultRoles:        a.cfg.Auth.OAuth2.DefaultRoles,
			EmailField:          a.cfg.Auth.OAuth2.EmailField,
			NameField:           a.cfg.Auth.OAuth2.NameField,
			PictureField:        a.cfg.Auth.OAuth2.PictureField,
			SubjectField:        a.cfg.Auth.OAuth2.SubjectField,
			RolesField:          a.cfg.Auth.OAuth2.RolesField,
			CookieName:          a.cfg.Auth.CookieName,
			AllowedDomains:      a.cfg.Auth.AllowedDomains,
			StateTTLSeconds:     a.cfg.Auth.StateTTLSeconds,
			TempCookieSecure:    a.cfg.Auth.CookieSecure,
			HTTPClient:          a.httpClient,
			DisablePKCE:         a.cfg.Auth.OAuth2.DisablePKCE,
		}
		oauthProvider, err := auth.NewOAuth2(ctx, a.authStore, opts)
		if err != nil {
			return fmt.Errorf("oauth2 init failed: %w", err)
		}
		a.authProvider = oauthProvider
	default:
		return fmt.Errorf("unsupported auth provider: %s", a.cfg.Auth.Provider)
	}
	return nil
}
