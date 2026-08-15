package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
	"agnos.dev/hospital-middleware/internal/patient"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StaffRepository struct {
	pool database
}

type database interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func NewStaffRepository(pool database) *StaffRepository {
	return &StaffRepository{pool: pool}
}

func (r *StaffRepository) Create(ctx context.Context, username, passwordHash, hospitalCode string) (core.Staff, error) {
	const query = `
		WITH selected_hospital AS (
			SELECT id, code FROM hospitals WHERE code = $3
		), inserted AS (
			INSERT INTO staff (hospital_id, username, password_hash)
			SELECT id, $1, $2 FROM selected_hospital
			RETURNING id, hospital_id, username, created_at
		)
		SELECT inserted.id, inserted.hospital_id, selected_hospital.code,
		       inserted.username, inserted.created_at
		FROM inserted
		JOIN selected_hospital ON selected_hospital.id = inserted.hospital_id`

	var created core.Staff
	err := r.pool.QueryRow(ctx, query, username, passwordHash, hospitalCode).Scan(
		&created.ID,
		&created.HospitalID,
		&created.HospitalCode,
		&created.Username,
		&created.CreatedAt,
	)
	if err == nil {
		return created, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Staff{}, fmt.Errorf("%w: hospital does not exist", core.ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return core.Staff{}, fmt.Errorf("%w: username already exists", core.ErrConflict)
	}
	return core.Staff{}, fmt.Errorf("create staff: %w", core.ErrInternal)
}

func (r *StaffRepository) GetByUsername(ctx context.Context, username string) (core.Staff, error) {
	const query = `
		SELECT staff.id, staff.hospital_id, hospitals.code, staff.username,
		       staff.password_hash, staff.created_at
		FROM staff
		JOIN hospitals ON hospitals.id = staff.hospital_id
		WHERE staff.username = $1`

	var found core.Staff
	err := r.pool.QueryRow(ctx, query, username).Scan(
		&found.ID,
		&found.HospitalID,
		&found.HospitalCode,
		&found.Username,
		&found.PasswordHash,
		&found.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Staff{}, core.ErrNotFound
	}
	if err != nil {
		return core.Staff{}, fmt.Errorf("get staff: %w", core.ErrInternal)
	}
	return found, nil
}

type PatientRepository struct {
	pool database
}

func NewPatientRepository(pool database) *PatientRepository {
	return &PatientRepository{pool: pool}
}

func (r *PatientRepository) Upsert(ctx context.Context, value core.Patient) error {
	const query = `
		INSERT INTO patients (
			hospital_id, first_name_th, middle_name_th, last_name_th,
			first_name_en, middle_name_en, last_name_en, date_of_birth,
			patient_hn, national_id, passport_id, phone_number, email, gender,
			source_system, source_updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14,
			$15, $16
		)
		ON CONFLICT (hospital_id, patient_hn) DO UPDATE SET
			first_name_th = EXCLUDED.first_name_th,
			middle_name_th = EXCLUDED.middle_name_th,
			last_name_th = EXCLUDED.last_name_th,
			first_name_en = EXCLUDED.first_name_en,
			middle_name_en = EXCLUDED.middle_name_en,
			last_name_en = EXCLUDED.last_name_en,
			date_of_birth = EXCLUDED.date_of_birth,
			national_id = EXCLUDED.national_id,
			passport_id = EXCLUDED.passport_id,
			phone_number = EXCLUDED.phone_number,
			email = EXCLUDED.email,
			gender = EXCLUDED.gender,
			source_system = EXCLUDED.source_system,
			source_updated_at = EXCLUDED.source_updated_at,
			updated_at = now()`

	_, err := r.pool.Exec(ctx, query,
		value.HospitalID,
		value.FirstNameTH,
		value.MiddleNameTH,
		value.LastNameTH,
		value.FirstNameEN,
		value.MiddleNameEN,
		value.LastNameEN,
		value.DateOfBirth,
		value.PatientHN,
		value.NationalID,
		value.PassportID,
		value.PhoneNumber,
		value.Email,
		value.Gender,
		value.SourceSystem,
		value.SourceUpdated,
	)
	if err != nil {
		return fmt.Errorf("upsert patient: %w", core.ErrInternal)
	}
	return nil
}

func (r *PatientRepository) Search(ctx context.Context, hospitalID string, criteria patient.Criteria, limit int) ([]core.Patient, error) {
	const query = `
		SELECT id, hospital_id,
		       first_name_th, middle_name_th, last_name_th,
		       first_name_en, middle_name_en, last_name_en,
		       date_of_birth, patient_hn,
		       COALESCE(national_id, ''), COALESCE(passport_id, ''),
		       COALESCE(phone_number, ''), COALESCE(email, ''), gender,
		       source_system, source_updated_at
		FROM patients
		WHERE hospital_id = $1
		  AND ($2 = '' OR national_id = $2)
		  AND ($3 = '' OR passport_id = $3)
		  AND ($4 = '' OR position(lower($4) in lower(first_name_th)) > 0 OR position(lower($4) in lower(first_name_en)) > 0)
		  AND ($5 = '' OR position(lower($5) in lower(middle_name_th)) > 0 OR position(lower($5) in lower(middle_name_en)) > 0)
		  AND ($6 = '' OR position(lower($6) in lower(last_name_th)) > 0 OR position(lower($6) in lower(last_name_en)) > 0)
		  AND ($7::date IS NULL OR date_of_birth = $7::date)
		  AND ($8 = '' OR phone_number = $8)
		  AND ($9 = '' OR lower(email) = lower($9))
		ORDER BY last_name_en, first_name_en, patient_hn
		LIMIT $10`

	var dateOfBirth any
	if criteria.DateOfBirth != nil {
		dateOfBirth = *criteria.DateOfBirth
	}

	rows, err := r.pool.Query(ctx, query,
		hospitalID,
		criteria.NationalID,
		criteria.PassportID,
		criteria.FirstName,
		criteria.MiddleName,
		criteria.LastName,
		dateOfBirth,
		criteria.PhoneNumber,
		criteria.Email,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search patients: %w", core.ErrInternal)
	}
	defer rows.Close()

	patients := make([]core.Patient, 0)
	for rows.Next() {
		var value core.Patient
		if err := rows.Scan(
			&value.ID,
			&value.HospitalID,
			&value.FirstNameTH,
			&value.MiddleNameTH,
			&value.LastNameTH,
			&value.FirstNameEN,
			&value.MiddleNameEN,
			&value.LastNameEN,
			&value.DateOfBirth,
			&value.PatientHN,
			&value.NationalID,
			&value.PassportID,
			&value.PhoneNumber,
			&value.Email,
			&value.Gender,
			&value.SourceSystem,
			&value.SourceUpdated,
		); err != nil {
			return nil, fmt.Errorf("scan patient: %w", core.ErrInternal)
		}
		patients = append(patients, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate patients: %w", core.ErrInternal)
	}
	return patients, nil
}

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	return pool, nil
}
