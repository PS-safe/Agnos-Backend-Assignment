# Entity-Relationship Diagram

```mermaid
erDiagram
    HOSPITALS ||--o{ STAFF : employs
    HOSPITALS ||--o{ PATIENTS : owns

    HOSPITALS {
        uuid id PK
        varchar code UK
        varchar name
        varchar his_adapter
        timestamptz created_at
        timestamptz updated_at
    }

    STAFF {
        uuid id PK
        uuid hospital_id FK
        varchar username UK
        varchar password_hash
        timestamptz created_at
        timestamptz updated_at
    }

    PATIENTS {
        uuid id PK
        uuid hospital_id FK
        varchar first_name_th
        varchar middle_name_th
        varchar last_name_th
        varchar first_name_en
        varchar middle_name_en
        varchar last_name_en
        date date_of_birth
        varchar patient_hn
        varchar national_id
        varchar passport_id
        varchar phone_number
        varchar email
        char gender
        varchar source_system
        timestamptz source_updated_at
        timestamptz created_at
        timestamptz updated_at
    }
```

## Constraints and tenant rules

- `hospitals.code` is the stable external/API identifier.
- `staff.username` is globally unique because login has no hospital input.
- Each staff member and patient has exactly one `hospital_id`.
- Patient access is always filtered by the `hospital_id` in the authenticated staff token.
- `(hospital_id, patient_hn)` is unique.
- `(hospital_id, national_id)` and `(hospital_id, passport_id)` are unique when the identifier is present.
- A patient must have at least one of `national_id` or `passport_id`.
- `gender` is constrained to `M` or `F` to match the stated Hospital A contract.

## Data flow

An identifier lookup goes to the hospital adapter. The normalized response is upserted using the hospital and HN identity, then queried again with every client-supplied filter. A non-identifier search queries previously normalized patient rows and is still limited to the authenticated hospital.

