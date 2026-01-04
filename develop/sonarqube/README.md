# SonarQube (Local)

This runs SonarQube locally for static analysis.

## Prerequisites

- Docker Desktop (or compatible Docker runtime)

## Configure credentials

`make sonar` reads credentials from `.env` (repo root). Add:

```
SONAR_HOST_PORT=19000
SONAR_PROJECT_KEY=manifold
SONAR_ADMIN_USER=admin
SONAR_ADMIN_PASSWORD=...your dev password...
```

## Run

- Start + scan: `make sonar`
- Start only: `make sonar-up`
- Stop: `make sonar-down`

UI:

- http://localhost:19000

## Files

- Compose stack: `develop/sonarqube/docker-compose.yml`
- Scanner config: `develop/sonarqube/sonar-project.properties`
