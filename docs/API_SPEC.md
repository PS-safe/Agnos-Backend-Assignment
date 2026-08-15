# API Specification

## Conventions

- Base URL in the Compose environment: `http://localhost:8080`
- Media type: `application/json`
- Dates: `YYYY-MM-DD`
- Timestamps: RFC 3339 UTC
- Protected routes use `Authorization: Bearer <access_token>`.
- Responses set `Cache-Control: no-store`.
- Error shape:

  ```json
  {
    "error": {
      "code": "invalid_request",
      "message": "request validation failed"
    }
  }
  ```

## `GET /health`

Process liveness endpoint.

### Success - `200 OK`

```json
{"status":"ok"}
```

## `POST /staff/create`

Creates login credentials for a staff member at an existing hospital.

> The route is public only because the assignment requires a creation API without an administrator contract. Restrict it before production use.

### Request

```json
{
  "username": "doctor.one",
  "password": "correct-horse-battery",
  "hospital": "hospital-a"
}
```

| Field | Rules |
|---|---|
| `username` | Required; normalized to lowercase; 3-64 ASCII letters, numbers, `.`, `_`, or `-`; globally unique |
| `password` | Required; valid UTF-8; 12-72 bytes |
| `hospital` | Required existing hospital code; normalized to lowercase |

Unknown JSON fields and multiple JSON values are rejected.

### Success - `201 Created`

```json
{
  "data": {
    "id": "d9e7ab1a-5b0e-4d89-92c2-86a6a94c133e",
    "username": "doctor.one",
    "hospital": "hospital-a",
    "created_at": "2026-08-15T10:00:00Z"
  }
}
```

### Errors

| Status | Code | Meaning |
|---|---|---|
| `400` | `invalid_request` | Invalid JSON, username, password, or hospital field |
| `404` | `not_found` | Hospital code does not exist |
| `409` | `conflict` | Username already exists |
| `500` | `internal_error` | Unexpected service failure |

## `POST /staff/login`

Authenticates a globally unique username and returns a hospital-scoped JWT.

### Request

```json
{
  "username": "doctor.one",
  "password": "correct-horse-battery"
}
```

### Success - `200 OK`

```json
{
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "token_type": "Bearer",
    "expires_in": 28800,
    "staff": {
      "id": "d9e7ab1a-5b0e-4d89-92c2-86a6a94c133e",
      "username": "doctor.one",
      "hospital": "hospital-a",
      "created_at": "2026-08-15T10:00:00Z"
    }
  }
}
```

### Errors

| Status | Code | Meaning |
|---|---|---|
| `400` | `invalid_request` | Invalid JSON shape |
| `401` | `unauthorized` | Username or password is invalid; the response does not reveal which |
| `500` | `internal_error` | Unexpected service failure |

## `GET /patient/search`

Searches patients belonging to the authenticated staff member's hospital. The hospital is taken from the verified token, not from the query string.

### Authorization

```text
Authorization: Bearer <access_token>
```

### Query parameters

Every field is individually optional, but at least one must be supplied. Unknown query parameters are rejected.

| Parameter | Match behavior |
|---|---|
| `national_id` | Exact; triggers Hospital A lookup when staff belongs to Hospital A |
| `passport_id` | Exact; triggers Hospital A lookup when `national_id` is absent |
| `first_name` | Case-insensitive substring across Thai and English first names |
| `middle_name` | Case-insensitive substring across Thai and English middle names |
| `last_name` | Case-insensitive substring across Thai and English last names |
| `date_of_birth` | Exact `YYYY-MM-DD` |
| `phone_number` | Exact normalized source value |
| `email` | Case-insensitive exact |

When both IDs are present, `national_id` is used for the upstream lookup and both IDs are applied to the normalized query. Results are limited to 100.

### Example

```http
GET /patient/search?national_id=1234567890123&last_name=Lovelace
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

### Success - `200 OK`

```json
{
  "data": {
    "patients": [
      {
        "first_name_th": "เอดา",
        "middle_name_th": "",
        "last_name_th": "เลิฟเลซ",
        "first_name_en": "Ada",
        "middle_name_en": "",
        "last_name_en": "Lovelace",
        "date_of_birth": "1815-12-10",
        "patient_hn": "HN-001",
        "national_id": "1234567890123",
        "passport_id": "",
        "phone_number": "0800000000",
        "email": "ada@example.com",
        "gender": "F"
      }
    ],
    "count": 1
  }
}
```

No match is a successful response with `patients: []` and `count: 0`.

### Errors

| Status | Code | Meaning |
|---|---|---|
| `400` | `invalid_request` | Empty search, unknown parameter, or malformed date |
| `401` | `unauthorized` | Missing, malformed, expired, or invalid access token |
| `502` | `hospital_unavailable` | Hospital HIS failed or returned an invalid payload |
| `500` | `internal_error` | Unexpected persistence or service failure |

## Hospital A upstream contract

The adapter calls:

```http
GET {HOSPITAL_A_BASE_URL}/patient/search/{national_id_or_passport_id}
Accept: application/json
X-API-Key: <optional HOSPITAL_A_API_KEY>
```

The expected body contains `first_name_th`, `middle_name_th`, `last_name_th`, `first_name_en`, `middle_name_en`, `last_name_en`, `date_of_birth`, `patient_hn`, `national_id`, `passport_id`, `phone_number`, `email`, and `gender`.

## Common transport behavior

| Status | Code | Meaning |
|---|---|---|
| `404` | `route_not_found` | Route does not exist |
| `413` or `400` | `invalid_request` | Request exceeds the one-megabyte body limit or is malformed |
| `429` | Nginx response | Per-client route rate limit exceeded |

