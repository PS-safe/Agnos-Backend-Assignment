# Agnos Hospital Middleware

A small hospital middleware service written in Go and Gin. It authenticates hospital staff, keeps every patient query within the authenticated staff member's hospital, integrates with Hospital A's identifier lookup, and stores normalized patient records in PostgreSQL.

## Deliverables

- [Consolidated reviewer deliverables PDF](output/pdf/Agnos_Backend_Deliverables.pdf)
- [Google Docs-ready planning document](docs/Agnos_Development_Planning.docx)
- [Development plan](docs/DEVELOPMENT_PLAN.md)
- [API specification](docs/API_SPEC.md)
- [OpenAPI 3.1 specification](docs/openapi.yaml)
- [ER diagram](docs/ER_DIAGRAM.md)

## Quick start with Docker Compose

Prerequisites: Docker with the Compose plugin.

1. Copy `.env.example` to `.env`.
2. Replace `POSTGRES_PASSWORD` and `JWT_SECRET` with strong values. `JWT_SECRET` must be at least 32 characters.
3. Start the stack:

   ```sh
   docker compose up --build
   ```

4. Check the service through Nginx:

   ```sh
   curl http://localhost:8080/health
   ```

The stack contains Nginx, the Go API, and PostgreSQL. The initial database migration is applied automatically when PostgreSQL creates a fresh data volume.

## Example workflow

The examples use synthetic data only.

Create a staff account:

```sh
curl -X POST http://localhost:8080/staff/create \
  -H "Content-Type: application/json" \
  -d '{"username":"doctor.one","password":"correct-horse-battery","hospital":"hospital-a"}'
```

Log in:

```sh
curl -X POST http://localhost:8080/staff/login \
  -H "Content-Type: application/json" \
  -d '{"username":"doctor.one","password":"correct-horse-battery","hospital":"hospital-a"}'
```

Use the returned access token to search:

```sh
curl "http://localhost:8080/patient/search?national_id=1234567890123" \
  -H "Authorization: Bearer REPLACE_WITH_ACCESS_TOKEN"
```

## Local development

The project targets Go 1.24. PostgreSQL must already contain the schema from `migrations/000001_init.up.sql`.

Required environment variables:

```text
DATABASE_URL=postgres://agnos:password@localhost:5432/agnos?sslmode=disable
JWT_SECRET=a-random-secret-containing-at-least-32-characters
```

Optional settings are documented in `.env.example`. Start the API with:

```sh
go run ./cmd/api
```

Run verification:

```sh
go test ./...
go test -race ./...
go vet ./...
```

Run the isolated Docker Compose smoke test:

```sh
./scripts/smoke-test.sh
```

The script uses a unique Compose project, listens on port `18080` by default, exercises health, authentication, validation, and PostgreSQL-backed patient search through Nginx, then removes its disposable containers and volume. Set `SMOKE_APP_PORT` to use a different host port.

## Architecture

```text
Client
  -> Nginx (routing, limits, request IDs)
    -> Gin HTTP handlers (validation and response mapping)
      -> Staff / patient application services
        -> PostgreSQL repositories
        -> Hospital client adapter
             -> Hospital A HIS
```

The service uses dependency inversion at its application boundaries, so handlers and business rules are tested with small fakes rather than a running database or external HIS.

## Key design decisions

- Staff usernames remain globally unique in the database. Login requires `username`, `password`, and `hospital`; the normalized hospital must match the account before a token is issued.
- Every individual patient filter is optional, but the request must contain at least one filter. Empty searches are rejected to prevent bulk patient enumeration.
- The hospital scope comes exclusively from the signed access token. It is never accepted from a patient-search query parameter.
- Hospital A supports exact national-ID or passport-ID lookup. An identifier search calls Hospital A, normalizes and upserts the result, and then applies all supplied criteria in PostgreSQL. Searches without an identifier query the normalized hospital cache.
- Results are capped at 100. Names use case-insensitive substring matching; IDs, date of birth, phone number, and email use exact matching.
- The brief does not specify Hospital A authentication. The adapter therefore supports an optional `HOSPITAL_A_API_KEY` sent as `X-API-Key`.
- The staff-creation route is public to match the required API. A production deployment should restrict it to an administrator or an invitation workflow.

## Security and privacy

- Passwords are not trimmed or normalized; they are validated as 12-72 UTF-8 bytes, then hashed with bcrypt cost 12. Passwords and hashes are never returned.
- JWTs use HS256, require issuer and expiration validation, and carry the authoritative staff and hospital IDs.
- Request logs record the route template and never log the patient-search query string, tokens, passwords, or response body.
- Nginx applies separate limits to authentication and search routes.
- API responses set `Cache-Control: no-store`.
- Keep the GitHub repository private and grant access only to the intended reviewers. Do not commit `.env`, credentials, assignment source files, or real patient data.

## Project layout

```text
cmd/api/                         application entry point
deploy/                          Nginx configuration
docs/                            planning, API, and ER deliverables
internal/config/                 environment configuration
internal/core/                   shared domain models and errors
internal/hospital/               Hospital A HIS adapter
internal/httpapi/                Gin routes, auth middleware, DTOs
internal/patient/                patient-search business rules
internal/security/               bcrypt and JWT handling
internal/staff/                  registration and login business rules
internal/storage/postgres/       PostgreSQL repositories
migrations/                      schema migrations and seed hospital
```

## Known production follow-ups

- Move secrets to a managed secret store and rotate signing keys.
- Add an administrator/invitation authorization policy for staff creation.
- Add audit events for patient access without recording patient values in application logs.
- Add retry/circuit-breaker behavior only after Hospital A's retry and rate-limit contract is known.
- Use a versioned migration runner for schema upgrades after the initial assignment deployment.
