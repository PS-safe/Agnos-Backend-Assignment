package postgres

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
	"agnos.dev/hospital-middleware/internal/patient"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

func TestStaffRepositoryCreate(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		mock.ExpectQuery("WITH selected_hospital").
			WithArgs("doctor.one", "password-hash", "hospital-a").
			WillReturnRows(pgxmock.NewRows([]string{"id", "hospital_id", "code", "username", "created_at"}).
				AddRow("staff-1", "hospital-id-a", "hospital-a", "doctor.one", createdAt))

		repository := NewStaffRepository(mock)
		created, err := repository.Create(context.Background(), "doctor.one", "password-hash", "hospital-a")
		if err != nil {
			t.Fatalf("create returned error: %v", err)
		}
		if created.ID != "staff-1" || created.HospitalID != "hospital-id-a" || created.HospitalCode != "hospital-a" {
			t.Fatalf("unexpected staff: %+v", created)
		}
		assertMockExpectations(t, mock)
	})

	t.Run("unknown hospital", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		mock.ExpectQuery("WITH selected_hospital").
			WithArgs("doctor.one", "password-hash", "missing-hospital").
			WillReturnRows(pgxmock.NewRows([]string{"id", "hospital_id", "code", "username", "created_at"}))

		repository := NewStaffRepository(mock)
		_, err := repository.Create(context.Background(), "doctor.one", "password-hash", "missing-hospital")
		if !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
		assertMockExpectations(t, mock)
	})

	t.Run("duplicate username", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		mock.ExpectQuery("WITH selected_hospital").
			WithArgs("doctor.one", "password-hash", "hospital-a").
			WillReturnError(&pgconn.PgError{Code: "23505"})

		repository := NewStaffRepository(mock)
		_, err := repository.Create(context.Background(), "doctor.one", "password-hash", "hospital-a")
		if !errors.Is(err, core.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
		assertMockExpectations(t, mock)
	})
}

func TestStaffRepositoryGetByUsername(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		mock.ExpectQuery("SELECT staff.id").
			WithArgs("doctor.one").
			WillReturnRows(pgxmock.NewRows([]string{"id", "hospital_id", "code", "username", "password_hash", "created_at"}).
				AddRow("staff-1", "hospital-id-a", "hospital-a", "doctor.one", "hash", createdAt))

		repository := NewStaffRepository(mock)
		found, err := repository.GetByUsername(context.Background(), "doctor.one")
		if err != nil {
			t.Fatalf("get returned error: %v", err)
		}
		if found.PasswordHash != "hash" || found.HospitalCode != "hospital-a" {
			t.Fatalf("unexpected staff: %+v", found)
		}
		assertMockExpectations(t, mock)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		mock.ExpectQuery("SELECT staff.id").
			WithArgs("missing").
			WillReturnRows(pgxmock.NewRows([]string{"id", "hospital_id", "code", "username", "password_hash", "created_at"}))

		repository := NewStaffRepository(mock)
		_, err := repository.GetByUsername(context.Background(), "missing")
		if !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
		assertMockExpectations(t, mock)
	})
}

func TestPatientRepositoryUpsert(t *testing.T) {
	t.Parallel()
	value := samplePatient()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		mock.ExpectExec("INSERT INTO patients").
			WithArgs(
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
			).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		repository := NewPatientRepository(mock)
		if err := repository.Upsert(context.Background(), value); err != nil {
			t.Fatalf("upsert returned error: %v", err)
		}
		assertMockExpectations(t, mock)
	})

	t.Run("database failure", func(t *testing.T) {
		t.Parallel()
		mock := newMockPool(t)
		mock.ExpectExec("INSERT INTO patients").
			WithArgs(
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
			).
			WillReturnError(errors.New("database unavailable"))

		repository := NewPatientRepository(mock)
		if err := repository.Upsert(context.Background(), value); !errors.Is(err, core.ErrInternal) {
			t.Fatalf("expected internal error, got %v", err)
		}
		assertMockExpectations(t, mock)
	})
}

func TestPatientRepositorySearch(t *testing.T) {
	t.Parallel()
	value := samplePatient()
	mock := newMockPool(t)
	queryPattern := regexp.QuoteMeta("SELECT id, hospital_id,")
	rows := pgxmock.NewRows([]string{
		"id", "hospital_id", "first_name_th", "middle_name_th", "last_name_th",
		"first_name_en", "middle_name_en", "last_name_en", "date_of_birth",
		"patient_hn", "national_id", "passport_id", "phone_number", "email",
		"gender", "source_system", "source_updated_at",
	}).AddRow(
		"patient-1", value.HospitalID, value.FirstNameTH, value.MiddleNameTH, value.LastNameTH,
		value.FirstNameEN, value.MiddleNameEN, value.LastNameEN, value.DateOfBirth,
		value.PatientHN, value.NationalID, value.PassportID, value.PhoneNumber, value.Email,
		value.Gender, value.SourceSystem, value.SourceUpdated,
	)
	mock.ExpectQuery(queryPattern).
		WithArgs("hospital-id-a", "1234567890123", "", "Ada", "", "", nil, "", "", 100).
		WillReturnRows(rows)

	repository := NewPatientRepository(mock)
	results, err := repository.Search(context.Background(), "hospital-id-a", patient.Criteria{
		NationalID: "1234567890123",
		FirstName:  "Ada",
	}, 100)
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "patient-1" || results[0].HospitalID != "hospital-id-a" {
		t.Fatalf("unexpected patients: %+v", results)
	}
	assertMockExpectations(t, mock)
}

func TestNewPoolRejectsInvalidURL(t *testing.T) {
	t.Parallel()
	if _, err := NewPool(context.Background(), "postgres://bad host/"); err == nil {
		t.Fatal("expected invalid database URL to fail")
	}
}

func samplePatient() core.Patient {
	return core.Patient{
		HospitalID:    "hospital-id-a",
		FirstNameTH:   "เอดา",
		LastNameTH:    "เลิฟเลซ",
		FirstNameEN:   "Ada",
		LastNameEN:    "Lovelace",
		DateOfBirth:   time.Date(1815, time.December, 10, 0, 0, 0, 0, time.UTC),
		PatientHN:     "HN-001",
		NationalID:    "1234567890123",
		PhoneNumber:   "0800000000",
		Email:         "ada@example.com",
		Gender:        "F",
		SourceSystem:  "hospital-a",
		SourceUpdated: time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC),
	}
}

func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("create pgx mock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	return mock
}

func assertMockExpectations(t *testing.T, mock pgxmock.PgxPoolIface) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet database expectations: %v", err)
	}
}
