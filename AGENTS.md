# Artemis API

Go HTTP API server (chi/v5 router). Serves the Artemis web frontend and mobile clients. Delegates scraping/enrichment to the Python worker via its internal API.

## Tech Stack
- **Go 1.26** (`go.mod`: `github.com/artemis/artemis-api`)
- **chi/v5** router
- **pgx/v5** for PostgreSQL (direct connection, not via worker)
- **pgvector-go** for vector similarity search
- **MinIO client** (`minio-go/v7`) for S3-compatible resume storage
- **JWT** (`golang-jwt/jwt/v5`) for auth

## Build & Run
```bash
go build -o api ./cmd/api
./api                    # listens on ${PORT:-8080}
```

Docker: two-stage build (`golang:1.23-alpine` → `alpine:3.20`) with `CGO_ENABLED=0`.

## Project Structure
```
cmd/api/main.go         -- Entry point
internal/
  config/config.go      -- Env vars: PORT, DATABASE_URL, JWT_SECRET, SCRAPER_API_URL, S3_*
  handler/              -- HTTP handlers (handler.go, auth_handler.go, user_handler.go)
  middleware/middleware.go -- CORS, JWT auth, API key auth, rate limiting
  model/models.go       -- Go structs mirroring DB schema
  repository/           -- company_repo.go, job_repo.go, user_repo.go (DB queries)
  service/              -- scraper_client.go (calls worker internal API), storage.go (MinIO)
migrations/             -- Empty (schema managed by worker's `artemis init-db`)
```

## Patterns
- All handlers receive dependencies via struct fields (no global state).
- JWT middleware validates tokens and injects user context.
- The API calls the Python worker's internal API for scraping, enrichment, and AI operations.
- Direct PostgreSQL reads for search/dashboard queries using pgx.

## Environment
`PORT`, `DATABASE_URL`, `JWT_SECRET`, `SCRAPER_API_URL` (worker internal API), `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`.

## Testing
```bash
./test_e2e.sh           # Full user-journey E2E test via curl
```
