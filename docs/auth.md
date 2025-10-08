# Authentication and RBAC

This project supports multi-user sign-in via OAuth2/OpenID Connect (OIDC) with simple RBAC backed by Postgres.

> Provisioning prerequisite: A user must first exist (or successfully authenticate) in the upstream Identity Provider (IdP) — e.g. Keycloak / Google / Okta — before you can grant elevated roles (like `admin`) inside this application. The first successful OIDC login creates (or upserts) the local user record; only then can an operator assign additional roles in Postgres.

## Overview

- OIDC provider (e.g., Google, Okta, Auth0) for user login
- Postgres tables for users, roles, user_roles, and sessions
- Session cookie (httpOnly) for auth. TTL configurable.
- Minimal RBAC with roles: `user` and `admin` by default

## Configuration

In `config.yaml` (or env), add an `Auth` section:

```yaml
Auth:
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

Also ensure `databases.defaultDSN` is set to your Postgres DSN.

### Using Keycloak (local dev)

This repo includes a Keycloak service in `docker-compose.yml`.

- Start infra: `docker compose up -d keycloak-db keycloak`
- Admin console: <http://localhost:8080> (admin / admin)
- A sample realm is auto-imported from `configs/keycloak/realm.json` with a client `agentd` and redirect `http://localhost:32180/*`.

Set the following in `config.yaml` to use Keycloak:

```yaml
Auth:
  enabled: true
  provider: oidc
  issuerURL: "http://localhost:8080/realms/sio-local"
  clientID: "agentd"
  clientSecret: "dev-agentd-secret"
  redirectURL: "http://localhost:32180/auth/callback"
  cookieName: "sio_session"
  cookieSecure: false
  stateTTLSeconds: 600
  sessionTTLHours: 72
```

## Endpoints & Auth Flow

| Method | Path           | Purpose |
|--------|----------------|---------|
| GET    | /auth/login    | Start OIDC Authorization Code + PKCE flow |
| GET    | /auth/callback | Complete code exchange, create session |
| GET    | /auth/logout   | Application + RP‑initiated IdP logout (ends SSO) |
| GET    | /api/me        | Current user JSON or 401 |

### Logout Semantics

The logout endpoint now performs **RP-initiated logout** against the IdP (Keycloak) so that:
 
1. The local session row (and httpOnly cookie) are deleted.
2. We redirect the browser to the IdP end-session endpoint with:
    - `client_id`
    - `post_logout_redirect_uri`
    - `id_token_hint` (retrieved server-side)
3. Keycloak clears the SSO session; user is returned to `/auth/login`.

Because we store the OIDC `id_token` **server-side in the `sessions` table** (column `id_token`), no extra browser cookie is required and the surface area for token exposure is reduced.

All API routes are protected when Auth is enabled. The UI assets redirect unauthenticated users to `/auth/login`.

## RBAC

Seed roles `admin` and `user` are created automatically. The callback assigns `user` by default. You can elevate users **after the user has logged in at least once** (so a row exists in `users`). Use:

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
- Allowed domains (optional): restrict initial login population by email domain.
- Chat history endpoints (`/api/chat/sessions*`) now scope results to the authenticated user. Admins continue to see all conversations, while standard users are limited to their own session IDs.
