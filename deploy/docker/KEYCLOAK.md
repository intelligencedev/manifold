# Keycloak for local auth (OIDC)

Use the included Keycloak + Postgres services in docker-compose for local login testing.

## Start services

```bash
docker compose up -d app-postgres keycloak-db keycloak
```

Admin console: <http://localhost:8083> (defaults: `admin` / `admin` for dev)

A sample realm is auto-imported from `configs/keycloak/realm.json` with client `agentd` and redirect `http://localhost:32180/*`.

## App configuration (config.yaml)

```yaml
auth:
  enabled: true
  provider: oidc
  issuerURL: "http://localhost:8083/realms/sio-local"
  clientID: agentd
  clientSecret: dev-agentd-secret
  redirectURL: "http://localhost:32180/auth/callback"
  cookieSecure: false # dev over HTTP
```

Then run the app and visit <http://localhost:32180/> — you should be redirected to Keycloak and returned after login.
