# Development Plan

## 1. Goal

Deliver a maintainable hospital middleware API that:

- creates hospital staff accounts and authenticates them;
- searches patients through a hospital-specific HIS integration;
- prevents a staff member from searching another hospital's patients;
- normalizes Hospital A data into a PostgreSQL-compatible patient model;
- runs behind Nginx with Docker Compose; and
- has positive and negative automated tests for every required API.

## 2. Scope and assumptions

### Included

- `POST /staff/create`
- `POST /staff/login`
- authenticated `GET /patient/search`
- Hospital A `GET /patient/search/{id}` adapter
- PostgreSQL hospital, staff, and patient schema
- JWT access tokens and bcrypt password hashes
- Nginx reverse proxy and route rate limits
- Docker Compose development/reviewer environment
- unit tests, OpenAPI, ER diagram, and operational README

### Explicit assumptions

1. `hospital` in staff creation is the stable hospital code. The seeded code is `hospital-a`.
2. Usernames remain globally unique, and login requires the hospital code to match the staff account before a token is issued.
3. Patient search fields are optional individually, but at least one must be supplied.
4. Hospital A can look up a patient only by national ID or passport ID. Searches by other fields operate on already normalized records for that hospital.
5. A Hospital A `404` is authoritative for an identifier lookup and produces an empty patient list.
6. Hospital A's response has the direct JSON shape given in the brief and uses `YYYY-MM-DD` dates and `M`/`F` gender values.
7. The upstream authentication contract is unspecified, so an optional `X-API-Key` is supported.
8. `POST /staff/create` remains public only to match the assignment. Production use requires administrator authorization.

## 3. Architecture

The code follows a small ports-and-adapters structure:

1. Gin handlers validate transport data and map errors to HTTP responses.
2. Staff and patient services enforce business and security rules.
3. Interfaces isolate password, token, persistence, and HIS boundaries.
4. PostgreSQL repositories implement durable storage.
5. The Hospital A adapter translates upstream JSON into the normalized patient model.

This separation keeps unit tests fast and makes another hospital adapter additive rather than invasive.

## 4. Implementation phases

### Phase 1 - Requirements and contracts

- Resolve ambiguous login and search behavior.
- Define endpoint request/response contracts and error codes.
- Define tenant boundary: hospital identity comes only from the authenticated token.
- Define normalized patient and staff models.

Exit condition: API and data-model decisions are documented and internally consistent.

### Phase 2 - Persistence and service skeleton

- Create PostgreSQL migration and seed Hospital A.
- Create config loader with startup validation.
- Add repository interfaces and PostgreSQL implementations.
- Add graceful HTTP startup and shutdown.

Exit condition: project compiles and the service can connect to a migrated database.

### Phase 3 - Authentication APIs

- Validate and normalize usernames and hospital codes.
- Hash passwords with bcrypt.
- Create staff accounts with conflict and unknown-hospital handling.
- Authenticate username, password, and hospital with a non-enumerating failure response.
- Issue and verify scoped JWT access tokens.

Exit condition: registration and login have positive and negative API tests.

### Phase 4 - Patient search

- Parse all supported search fields.
- Reject empty or unknown-filter searches.
- Derive hospital scope from JWT claims.
- Call Hospital A for ID searches.
- Normalize/upsert HIS results and query by all supplied criteria.
- Cap results and map upstream failures without leaking details.

Exit condition: search, unauthorized, invalid, not-found, and upstream-failure paths are tested.

### Phase 5 - Runtime packaging

- Build a non-root, multi-stage Go container.
- Add PostgreSQL and migration bootstrap to Compose.
- Add Nginx routing, request IDs, body cap, timeouts, and rate limits.
- Supply a safe `.env.example` and ignore real secrets.

Exit condition: `docker compose config` is valid and the stack starts in an environment with Docker.

### Phase 6 - Verification and handoff

- Run `gofmt`, `go test`, race detection, coverage, and `go vet`.
- Validate YAML and OpenAPI syntax.
- Review logs and responses for sensitive-data leakage.
- Complete the README, API specification, and ER diagram.

Exit condition: checks pass, limitations are documented, and the repository is private-ready.

## 5. Test strategy

| Layer | Test focus |
|---|---|
| HTTP handlers | status codes, request validation, JSON shape, authentication requirement, error mapping |
| Staff service | normalization, password policy, safe credential failures, removal of password hashes |
| Patient service | authoritative hospital scoping, upstream lookup, cache hydration, empty/not-found behavior |
| Hospital A adapter | URL/header contract, JSON mapping, normalization, status and malformed-payload handling |
| Token manager | issue/verify round trip and tamper rejection |

PostgreSQL integration tests are a production follow-up; repository SQL is kept small and all hospital scoping is visible in the queries.

## 6. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Cross-hospital data disclosure | Ignore client-supplied hospital data; scope every patient query by authenticated `hospital_id` |
| Empty search enumerates patients | Require at least one criterion and cap results at 100 |
| Upstream HIS is slow or unavailable | Strict HTTP timeout and generic `502 hospital_unavailable` response |
| Credential enumeration | Use the same `401` response for unknown user and wrong password |
| Sensitive data in logs | Log route templates, never raw query strings, bodies, tokens, or patient responses |
| Duplicate hospital identities | Hospital-scoped unique indexes for HN, national ID, and passport ID |
| Assignment material leaks | Keep repository private and exclude source PDF, `.env`, and real data |

## 7. Definition of done

- All required endpoints and fields are implemented.
- Hospital scoping is enforced from a verified token.
- Positive and negative API tests pass.
- Go formatting, tests, race detector, and vet pass.
- Docker Compose includes Nginx, Go, and PostgreSQL.
- Development plan, API spec, ER diagram, and README are complete.
- Repository contains no secrets, source assignment file, or real patient data.
