# Authentication and RBAC

This project supports multi-user sign-in via OpenID Connect (OIDC) or plain OAuth2 with simple RBAC backed by Postgres.

> Provisioning prerequisite: A user must first exist (or successfully authenticate) in the upstream Identity Provider (IdP) — e.g. Keycloak / Google / Okta — before you can grant elevated roles (like `admin`) inside this application. The first successful OIDC login creates (or upserts) the local user record; only then can an operator assign additional roles in Postgres.

## Overview

- OIDC provider (e.g., Google, Okta, Auth0) for user login
- Postgres tables for users, roles, user_roles, and sessions
- Session cookie (httpOnly) for auth. TTL configurable.
- Minimal RBAC with roles: `user` and `admin` by default

## Configuration

Configure authentication in `config.yaml`. If you want to keep secrets out of the file, reference environment variables with `${VAR}` and define those in `.env` or the host environment.

Add an `auth` section like this:

```yaml
auth:
  enabled: true
  provider: oidc
  issuerURL: "https://accounts.google.com" # or your provider
  clientID: "${OIDC_CLIENT_ID}"
  clientSecret: "${OIDC_CLIENT_SECRET}"
  redirectURL: "http://localhost:32180/auth/callback"
  allowedDomains: ["example.com"]
  cookieName: "sio_session"
  cookieSecure: false  # set true in production (HTTPS)
  cookieDomain: ""
  stateTTLSeconds: 600
  sessionTTLHours: 72
```

Also ensure `databases.defaultDSN` is set in `config.yaml`, typically via `${DATABASE_URL}`.

### Using Keycloak (local dev)

This repo includes a Keycloak service in `docker-compose.yml`.

- Start infra: `docker compose up -d keycloak-db keycloak`
- Admin console: <http://localhost:8083> (admin / admin)
- A sample realm is auto-imported from `deploy/configs/keycloak/realm.json` into the `keycloak` service with a client `agentd` and redirect `http://localhost:32180/*`.

Set the following in `config.yaml` to use Keycloak:

```yaml
auth:
  enabled: true
  provider: oidc
  issuerURL: "http://localhost:8083/realms/sio-local"
  clientID: "agentd"
  clientSecret: "${AUTH_CLIENT_SECRET}"
  redirectURL: "http://localhost:32180/auth/callback"
  cookieName: "sio_session"
  cookieSecure: false
  stateTTLSeconds: 600
  sessionTTLHours: 72
```

For local dev, put `AUTH_CLIENT_SECRET="dev-agentd-secret"` in `.env`.

## Provider Examples

Use OIDC providers when possible. Manifold discovers OIDC endpoints from `issuerURL`, verifies the ID token, and stores a local httpOnly session cookie. Use `provider: oauth2` for providers that do not expose a compatible OIDC discovery document or when you need explicit endpoint and JSON-field mapping.

### Google

Google supports OIDC discovery at `https://accounts.google.com/.well-known/openid-configuration`. Register an OAuth web application in Google Cloud Console and add Manifold's callback URL as an authorized redirect URI.

```yaml
auth:
  enabled: true
  provider: oidc
  issuerURL: "https://accounts.google.com"
  clientID: "${GOOGLE_CLIENT_ID}"
  clientSecret: "${GOOGLE_CLIENT_SECRET}"
  redirectURL: "http://localhost:32180/auth/callback"
  allowedDomains:
    - example.com
  cookieName: "manifold_session"
  cookieSecure: false
  cookieDomain: ""
  stateTTLSeconds: 600
  sessionTTLHours: 72
```

Notes:

- Set `allowedDomains: []` to allow any Google account.
- In production, use an HTTPS redirect URL and set `cookieSecure: true`.
- Google account identity should be keyed by the OIDC `sub` claim. Manifold already stores the verified ID token subject as `users.subject`.
- Google does not provide a browser RP-initiated logout endpoint. Manifold logout clears the local session and redirects locally unless `auth.oidc.logoutURL` is configured.

### GitHub

GitHub OAuth Apps use explicit OAuth2 endpoints rather than Manifold's OIDC path. Create a GitHub OAuth App and set its authorization callback URL to Manifold's callback URL.

```yaml
auth:
  enabled: true
  provider: oauth2
  clientID: "${GITHUB_CLIENT_ID}"
  clientSecret: "${GITHUB_CLIENT_SECRET}"
  redirectURL: "http://localhost:32180/auth/callback"
  allowedDomains: []
  cookieName: "manifold_session"
  cookieSecure: false
  cookieDomain: ""
  stateTTLSeconds: 600
  sessionTTLHours: 72
  oauth2:
    authURL: "https://github.com/login/oauth/authorize"
    tokenURL: "https://github.com/login/oauth/access_token"
    userInfoURL: "https://api.github.com/user"
    scopes:
      - read:user
      - user:email
    providerName: "github"
    defaultRoles:
      - user
    emailField: "email"
    nameField: "name"
    pictureField: "avatar_url"
    subjectField: "id"
    rolesField: ""
    disablePKCE: false
```

Notes:

- GitHub users can hide their public email address. Manifold's generic OAuth2 user-info mapper currently reads `email` from `https://api.github.com/user`; if that field is empty, login fails with `email required`. Supporting private GitHub email addresses requires adding provider-specific lookup against GitHub's email API and selecting the primary verified address.
- Keep `disablePKCE: false` unless you are integrating a provider or app type that rejects PKCE.

### Apple

Apple supports OIDC discovery at `https://appleid.apple.com/.well-known/openid-configuration`, but its web flow has provider-specific requirements. Manifold supports these through the `auth.oidc` block:

- Apple scopes are `openid`, `email`, and `name`.
- Apple requires `response_mode=form_post` when requesting user scopes.
- Apple's token endpoint expects client credentials in the form body, so set `tokenAuthStyle: "params"`.
- Apple uses an ES256 client-secret JWT. Manifold can generate it from your Apple Team ID, Services ID, Key ID, and private key.

```yaml
auth:
  enabled: true
  provider: oidc
  issuerURL: "https://appleid.apple.com"
  clientID: "${APPLE_SERVICE_ID}"
  redirectURL: "https://manifold.example.com/auth/callback"
  allowedDomains: []
  cookieName: "manifold_session"
  cookieSecure: true
  cookieDomain: ""
  stateTTLSeconds: 600
  sessionTTLHours: 72
  oidc:
    scopes:
      - openid
      - email
      - name
    responseMode: "form_post"
    tokenAuthStyle: "params"
    providerName: "apple"
    apple:
      teamID: "${APPLE_TEAM_ID}"
      keyID: "${APPLE_KEY_ID}"
      privateKeyPath: "${APPLE_PRIVATE_KEY_PATH}"
      # Or provide the PEM content directly through an environment variable:
      # privateKey: "${APPLE_PRIVATE_KEY}"
      clientSecretTTLHours: 4320
```

Notes:

- `APPLE_SERVICE_ID` is the Services ID registered for Sign in with Apple.
- `clientSecretTTLHours` defaults to Apple's six-month maximum and is capped to that maximum.
- Apple requires HTTPS redirect URIs for web flows and does not allow localhost redirect URLs for a Services ID. For local testing, use a real HTTPS development hostname or a tunnel registered in the Apple developer portal.
- Apple only sends the user's name on the first authorization. Manifold stores that first value when Apple provides it and preserves existing local profile data when later callbacks omit it.
- Apple logout clears Manifold's local session. Apple does not expose a Keycloak-style browser logout endpoint.

### Plain OAuth2 Template

Set `provider: oauth2` and describe the authorization/token/user info endpoints manually:

```yaml
auth:
  enabled: true
  provider: oauth2
  clientID: "${GITHUB_CLIENT_ID}"
  clientSecret: "${GITHUB_CLIENT_SECRET}"
  redirectURL: "http://localhost:32180/auth/callback"
  oauth2:
    authURL: "https://github.com/login/oauth/authorize"
    tokenURL: "https://github.com/login/oauth/access_token"
    userInfoURL: "https://api.github.com/user"
    scopes: ["read:user", "user:email"]
    providerName: "github"
    emailField: "email"
    nameField: "name"
    pictureField: "avatar_url"
    subjectField: "id"
    rolesField: ""          # optional JSON array of extra roles
    defaultRoles: ["user"]  # automatically applied when the IdP does not send roles
```

The OAuth2 block tells Manifold how to exchange codes and which JSON fields to read from the user info response. `subjectField` must resolve to a stable identifier (falling back to `email` if left blank). `rolesField`, when present, should point at an array of strings; the values are synchronized into the RBAC table in addition to `defaultRoles`. Logout simply clears the local session unless `logoutURL` is provided (paired with `logoutRedirectParam`), in which case the browser is redirected to the upstream IdP after the local cookie is deleted.

## Endpoints & Auth Flow

| Method | Path | Purpose |
| --- | --- | --- |
| GET | /auth/login | Start OIDC or OAuth2 Authorization Code + PKCE flow |
| GET | /auth/callback | Complete query-mode code exchange, create session |
| POST | /auth/callback | Complete `form_post` code exchange, used by providers such as Apple |
| GET | /auth/logout | Clear the local session; may also redirect to the upstream IdP logout endpoint |
| GET | /api/me | Current user JSON or 401 |

### Logout Semantics

The logout endpoint always deletes the local Manifold session row and clears the httpOnly session cookie. Upstream IdP logout behavior depends on provider configuration:

- The plain OAuth2 provider redirects to `oauth2.logoutURL` when configured; otherwise it completes local logout only.
- The OIDC provider redirects to `oidc.logoutURL` when configured.
- For backward compatibility with the bundled Keycloak setup, the OIDC provider still derives a Keycloak end-session URL from issuer URLs containing `/realms/`.
- Providers such as Google and Apple normally complete local logout only.

For Keycloak, OIDC logout performs **RP-initiated logout** so that:

1. The local session row (and httpOnly cookie) are deleted.
2. We redirect the browser to the IdP end-session endpoint with:
    - `client_id`
    - `post_logout_redirect_uri`
    - `id_token_hint` (retrieved server-side)
3. Keycloak clears the SSO session; user is returned to `/auth/login`.

Because we store the OIDC `id_token` **server-side in the `sessions` table** (column `id_token`), no extra browser cookie is required and the surface area for token exposure is reduced.

All API routes are protected when Auth is enabled. The UI assets redirect unauthenticated users to `/auth/login`.

## RBAC

Seed roles `admin` and `user` are created automatically. The OIDC callback always grants `user` and promotes to `admin` when the identity provider asserts that role (either as a realm role or a group named `admin`). For other roles you can still elevate users **after the user has logged in at least once** (so a row exists in `users`). Use:

```sql
INSERT INTO user_roles(user_id, role_id)
SELECT u.id, r.id FROM users u, roles r
WHERE u.email='alice@example.com' AND r.name='admin'
ON CONFLICT DO NOTHING;
```

Then wrap sensitive routes with `auth.RequireRoles(store, "admin")` (or explicit checks using `HasRole`) for admin-only APIs.

### Role Assignment Workflow Summary

1. Ensure user exists in IdP (create them or let them self-register depending on IdP policy).
2. User logs in once → local `users` + `sessions` row created, assigned role `user`.
3. Admin runs SQL (or future admin UI) to grant `admin` role.
4. User re-authenticates / refreshes – elevated privileges now effective.

### Security Notes

- Session cookie: httpOnly, SameSite=Lax (configure `cookieSecure` + `cookieDomain` for production).
- ID token: persisted server-side only (`sessions.id_token`) to support RP-initiated logout; never exposed via API.
- Logout: always a top-level navigation so browser follows IdP redirect chain; avoids stale SSO sessions.
- The auth loader is YAML-first. `.env` values only matter when referenced from `config.yaml` via `${VAR}`.
- Allowed domains (optional): restrict initial login population by email domain.
- Chat history endpoints (`/api/chat/sessions*`) now scope results to the authenticated user. Admins continue to see all conversations, while standard users are limited to their own session IDs.
- OIDC logins synchronise the `admin` role automatically; users must log out and back in for role changes at the identity provider to take effect. Ensure your IdP includes realm roles or groups in the ID token (e.g. in Keycloak add a group/role mapper to the `agentd` client) so the callback can observe them.
