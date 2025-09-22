# Authentication and RBAC

This project supports multi-user sign-in via OAuth2/OpenID Connect (OIDC) with simple RBAC backed by Postgres.

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

## Endpoints

- `GET /auth/login` starts the OIDC flow
- `GET /auth/callback` completes the login
- `POST /auth/logout` logs out and clears cookie
- `GET /api/me` returns current user or 401

All API routes are protected when Auth is enabled. The UI assets redirect unauthenticated users to `/auth/login`.

## RBAC

Seed roles `admin` and `user` are created automatically. The callback assigns `user` by default. You can elevate users with:

```sql
INSERT INTO user_roles(user_id, role_id)
SELECT u.id, r.id FROM users u, roles r
WHERE u.email='alice@example.com' AND r.name='admin'
ON CONFLICT DO NOTHING;
```

Then wrap sensitive routes with `auth.RequireRoles(store, "admin")` if you need admin-only APIs.
