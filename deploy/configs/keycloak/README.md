# Keycloak realm files

Keycloak realm import files live here.

For local development, a sample realm `realm.json` can be provided to auto-import on startup:

- Client ID: `agentd`
- Redirect URIs: `http://localhost:32180/*`
- Web origins: `http://localhost:32180`

Keycloak in the repository root compose file is exposed at <http://localhost:8083>.

Default admin: `admin` / `admin` (dev only)
