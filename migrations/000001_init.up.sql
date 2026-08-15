CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE hospitals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(64) NOT NULL UNIQUE,
    name varchar(255) NOT NULL,
    his_adapter varchar(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT hospitals_code_format CHECK (code ~ '^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$')
);

CREATE TABLE staff (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id uuid NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    username varchar(64) NOT NULL UNIQUE,
    password_hash varchar(255) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT staff_username_normalized CHECK (username = lower(username)),
    CONSTRAINT staff_username_format CHECK (username ~ '^[a-z0-9._-]{3,64}$')
);

CREATE INDEX staff_hospital_id_idx ON staff (hospital_id);

CREATE TABLE patients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id uuid NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    first_name_th varchar(255) NOT NULL DEFAULT '',
    middle_name_th varchar(255) NOT NULL DEFAULT '',
    last_name_th varchar(255) NOT NULL DEFAULT '',
    first_name_en varchar(255) NOT NULL DEFAULT '',
    middle_name_en varchar(255) NOT NULL DEFAULT '',
    last_name_en varchar(255) NOT NULL DEFAULT '',
    date_of_birth date NOT NULL,
    patient_hn varchar(128) NOT NULL,
    national_id varchar(64),
    passport_id varchar(64),
    phone_number varchar(64),
    email varchar(320),
    gender char(1) NOT NULL,
    source_system varchar(64) NOT NULL,
    source_updated_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT patients_gender_check CHECK (gender IN ('M', 'F')),
    CONSTRAINT patients_identifier_check CHECK (national_id IS NOT NULL OR passport_id IS NOT NULL),
    CONSTRAINT patients_hospital_hn_unique UNIQUE (hospital_id, patient_hn)
);

CREATE UNIQUE INDEX patients_hospital_national_id_unique
    ON patients (hospital_id, national_id)
    WHERE national_id IS NOT NULL;

CREATE UNIQUE INDEX patients_hospital_passport_id_unique
    ON patients (hospital_id, passport_id)
    WHERE passport_id IS NOT NULL;

CREATE INDEX patients_hospital_name_idx
    ON patients (hospital_id, lower(last_name_en), lower(first_name_en));

CREATE INDEX patients_hospital_birth_date_idx
    ON patients (hospital_id, date_of_birth);

INSERT INTO hospitals (code, name, his_adapter)
VALUES ('hospital-a', 'Hospital A', 'hospital-a')
ON CONFLICT (code) DO NOTHING;

