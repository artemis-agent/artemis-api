# artemis-api

Go HTTP API for [Artemis](https://github.com/artemis-agent), a job platform that scrapes openings directly from company applicant tracking systems (ATS) and ranks them against your resume. This service is the public backend for [artemis-web](https://github.com/artemis-agent/artemis-web): it handles auth, job and company queries, saved jobs, and resume uploads, and delegates AI and scraping work to [artemis-worker](https://github.com/artemis-agent/artemis-worker).

![Artemis dashboard served by this API: job recommendations with match scores](docs/screenshots/dashboard.png)

## What it does

- **Auth**: register, login, JWT sessions, onboarding, and password reset via a security question.
- **Jobs**: list, filter, and fetch jobs from PostgreSQL. Semantic search embeds the query through the worker, then ranks jobs by pgvector cosine distance.
- **Companies**: list companies, company detail, and a company's jobs.
- **Users**: profile and preferences, resume upload to S3-compatible storage (MinIO), resume parse progress, and recommendations (computed by the worker).
- **Saved jobs**: save, unsave, list.
- **Hunt**: ask the worker to find a new company's careers page and add it.

## Where it fits

```text
artemis-web --> artemis-api (this repo) --HTTP--> artemis-worker internal API (:8081)
                      |                                  |
                      +------ PostgreSQL + pgvector -----+
                      +------ MinIO (resumes) -----------+
```

The API reads PostgreSQL directly for search and dashboard queries. It calls the worker's internal API (`SCRAPER_API_URL`) for embeddings, recommendations, resume parsing, scrape triggers, and company hunts. The database schema is created by the worker (`artemis init-db`), not by this repo.

## Stack

Go, [chi](https://github.com/go-chi/chi) router, pgx/v5 with pgvector-go, golang-jwt, minio-go, `log/slog`.

## Run locally

Prerequisites: Go (version in `go.mod`), and the services from artemis-worker running: PostgreSQL with pgvector, MinIO, and the worker's internal API. The worker repo's `docker-compose.yml` starts all of them, and `artemis init-db` creates the schema.

```bash
go run ./cmd/api          # listens on :8080
```

Or build a binary:

```bash
go build -o api ./cmd/api
./api
```

A `Dockerfile` is included (multi-stage build, exposes 8080, health check on `/healthz`).

### Configuration

All settings are environment variables (`internal/config/config.go`). The defaults match the worker's docker-compose services.

| Variable | Default | Purpose |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port |
| `DATABASE_URL` | `postgres://artemis:artemis@localhost:5432/artemis?sslmode=disable&pool_max_conns=20` | PostgreSQL connection |
| `JWT_SECRET` | dev-only placeholder | JWT signing key; set this outside local dev |
| `API_KEY` | empty | Required as `X-API-Key` on admin routes; empty disables the check |
| `ALLOWED_ORIGINS` | `http://localhost:3000` | CORS origins |
| `SCRAPER_API_URL` | `http://localhost:8081` | artemis-worker internal API |
| `S3_ENDPOINT` | `http://localhost:9000` | MinIO / S3 endpoint |
| `S3_ACCESS_KEY` | `artemis` | S3 access key |
| `S3_SECRET_KEY` | `artemis123` | S3 secret key |
| `S3_BUCKET` | `artemis-resumes` | Bucket for resume files |

### Testing

`test_e2e.sh` walks a full user journey (health, register, login, onboarding, search, saved jobs, and more) with `curl` and `jq` against a running server at `localhost:8080`:

```bash
./test_e2e.sh
```

## Routes

Health: `GET /healthz`, `GET /readyz`. Everything else is under `/api/v1`:

| Access | Routes |
| --- | --- |
| Public | `POST /auth/register`, `/auth/login`, `/auth/forgot-password`, `/auth/verify-security`, `/auth/reset-password`; `GET /public/jobs` (previews) |
| API key | `POST /companies/{slug}/request` (rate limited) |
| JWT | `GET /auth/me`; `POST /auth/onboarding`, `/auth/security-question`; `GET /jobs`, `/jobs/search`, `/jobs/{id}`; `GET /companies`, `/companies/{slug}`, `/companies/{slug}/jobs`; `GET /stats`; `GET/PUT /users/{id}`; `PUT /users/{id}/preferences`; `POST/GET /users/{id}/resume`; `GET /users/{id}/resume/progress`, `/users/{id}/recommendations`; `GET /saved`, `/saved/ids`; `POST/DELETE /saved/{jobId}`; `POST /hunt` |

See `cmd/api/main.go` for the authoritative list.

## Project layout

```text
cmd/api/main.go        entry point, router, graceful shutdown
internal/config/       environment config
internal/handler/      HTTP handlers (jobs, companies, auth, users)
internal/middleware/   CORS, logging, JWT auth, API key auth, rate limiting
internal/model/        structs mirroring the DB schema
internal/repository/   PostgreSQL queries (jobs, companies, users)
internal/service/      worker internal-API client, MinIO storage
```

## Related repos

| Repo | Role |
| --- | --- |
| [artemis-web](https://github.com/artemis-agent/artemis-web) | Next.js frontend |
| [artemis-api](https://github.com/artemis-agent/artemis-api) | Go HTTP API (this repo) |
| [artemis-worker](https://github.com/artemis-agent/artemis-worker) | Python engine: company discovery, ATS detection, scraping, enrichment, recommendations |
