package agentd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"manifold/internal/auth"
	"manifold/internal/config"
	"manifold/internal/persistence/databases"
)

func (a *app) initAuth(ctx context.Context) error {
	if !a.cfg.Auth.Enabled {
		return nil
	}

	dsn := a.cfg.Databases.DefaultDSN
	if dsn == "" && a.mgr != nil && a.mgr.SQLite != nil {
		a.authStore = auth.NewSQLiteStore(a.mgr.SQLite, a.cfg.Auth.SessionTTLHours)
		if err := a.authStore.InitSchema(ctx); err != nil {
			return fmt.Errorf("auth sqlite schema init failed: %w", err)
		}
		_ = a.authStore.EnsureDefaultRoles(ctx)
		return a.initConfiguredAuthProvider(ctx)
	}
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

	return a.initConfiguredAuthProvider(ctx)
}

func (a *app) initConfiguredAuthProvider(ctx context.Context) error {
	providerName := strings.ToLower(strings.TrimSpace(a.cfg.Auth.Provider))
	if providerName == "" {
		providerName = "oidc"
	}
	switch providerName {
	case "oidc":
		return a.initOIDCAuth(ctx)
	case "oauth2":
		return a.initOAuth2Auth(ctx)
	default:
		return fmt.Errorf("unsupported auth provider: %s", a.cfg.Auth.Provider)
	}
}

func (a *app) initOIDCAuth(ctx context.Context) error {
	if strings.TrimSpace(a.cfg.Auth.IssuerURL) == "" {
		return fmt.Errorf("auth.provider=oidc requires issuerURL")
	}
	clientSecret, err := a.oidcClientSecret()
	if err != nil {
		return err
	}
	oidcAuth, err := auth.NewOIDC(ctx, auth.OIDCOptions{
		IssuerURL:           a.cfg.Auth.IssuerURL,
		ClientID:            a.cfg.Auth.ClientID,
		ClientSecret:        clientSecret,
		RedirectURL:         a.cfg.Auth.RedirectURL,
		Store:               a.authStore,
		CookieName:          a.cfg.Auth.CookieName,
		AllowedDomains:      a.cfg.Auth.AllowedDomains,
		StateTTLSeconds:     a.cfg.Auth.StateTTLSeconds,
		TempCookieSecure:    a.cfg.Auth.CookieSecure,
		Scopes:              a.cfg.Auth.OIDC.Scopes,
		ResponseMode:        a.cfg.Auth.OIDC.ResponseMode,
		TokenAuthStyle:      oidcTokenAuthStyle(a.cfg.Auth.OIDC.TokenAuthStyle),
		ProviderName:        a.cfg.Auth.OIDC.ProviderName,
		LogoutURL:           a.cfg.Auth.OIDC.LogoutURL,
		LogoutRedirectParam: a.cfg.Auth.OIDC.LogoutRedirectParam,
	})
	if err != nil {
		return fmt.Errorf("oidc init failed: %w", err)
	}
	a.authProvider = oidcAuth
	return nil
}

func (a *app) initOAuth2Auth(ctx context.Context) error {
	oauthProvider, err := auth.NewOAuth2(ctx, a.authStore, auth.OAuth2Options{
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
	})
	if err != nil {
		return fmt.Errorf("oauth2 init failed: %w", err)
	}
	a.authProvider = oauthProvider
	return nil
}

func (a *app) oidcClientSecret() (string, error) {
	if strings.TrimSpace(a.cfg.Auth.ClientID) == "" || strings.TrimSpace(a.cfg.Auth.ClientSecret) == "" {
		if !hasAppleClientSecretConfig(a.cfg.Auth.OIDC.Apple) {
			return "", fmt.Errorf("auth.provider=oidc requires clientID and clientSecret")
		}
	}
	if !hasAppleClientSecretConfig(a.cfg.Auth.OIDC.Apple) {
		return a.cfg.Auth.ClientSecret, nil
	}
	secret, err := auth.GenerateAppleClientSecret(auth.AppleClientSecretOptions{
		TeamID:         a.cfg.Auth.OIDC.Apple.TeamID,
		KeyID:          a.cfg.Auth.OIDC.Apple.KeyID,
		ClientID:       a.cfg.Auth.ClientID,
		PrivateKey:     a.cfg.Auth.OIDC.Apple.PrivateKey,
		PrivateKeyPath: a.cfg.Auth.OIDC.Apple.PrivateKeyPath,
		TTL:            time.Duration(a.cfg.Auth.OIDC.Apple.ClientSecretTTLHours) * time.Hour,
	})
	if err != nil {
		return "", fmt.Errorf("apple client secret generation failed: %w", err)
	}
	return secret, nil
}

func hasAppleClientSecretConfig(cfg config.AppleOIDCConfig) bool {
	return strings.TrimSpace(cfg.TeamID) != "" ||
		strings.TrimSpace(cfg.KeyID) != "" ||
		strings.TrimSpace(cfg.PrivateKey) != "" ||
		strings.TrimSpace(cfg.PrivateKeyPath) != ""
}

func oidcTokenAuthStyle(value string) oauth2.AuthStyle {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "params", "in_params", "inparams":
		return oauth2.AuthStyleInParams
	case "header", "basic", "in_header", "inheader":
		return oauth2.AuthStyleInHeader
	default:
		return 0
	}
}
