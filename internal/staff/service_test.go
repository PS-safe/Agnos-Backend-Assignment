package staff

import (
	"context"
	"errors"
	"testing"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
)

type fakeStaffRepository struct {
	createUsername string
	createHash     string
	createHospital string
	created        core.Staff
	found          core.Staff
	err            error
}

func (f *fakeStaffRepository) Create(_ context.Context, username, passwordHash, hospitalCode string) (core.Staff, error) {
	f.createUsername = username
	f.createHash = passwordHash
	f.createHospital = hospitalCode
	return f.created, f.err
}

func (f *fakeStaffRepository) GetByUsername(_ context.Context, username string) (core.Staff, error) {
	f.createUsername = username
	return f.found, f.err
}

type fakePasswords struct {
	hash       string
	compareErr error
}

func (f fakePasswords) Hash(string) (string, error)  { return f.hash, nil }
func (f fakePasswords) Compare(string, string) error { return f.compareErr }

type fakeTokens struct {
	token     string
	expiresAt time.Time
}

func (f fakeTokens) Issue(core.Staff) (string, time.Time, error) {
	return f.token, f.expiresAt, nil
}

func TestCreateNormalizesUsernameAndHospital(t *testing.T) {
	t.Parallel()
	repository := &fakeStaffRepository{created: core.Staff{
		ID:           "staff-1",
		Username:     "doctor.one",
		HospitalCode: "hospital-a",
		PasswordHash: "should-not-leak",
	}}
	service := NewService(repository, fakePasswords{hash: "hashed-password"}, fakeTokens{})

	created, err := service.Create(context.Background(), CreateInput{
		Username:     " Doctor.One ",
		Password:     "correct-horse-battery",
		HospitalCode: " Hospital-A ",
	})
	if err != nil {
		t.Fatalf("create returned error: %v", err)
	}
	if repository.createUsername != "doctor.one" || repository.createHospital != "hospital-a" {
		t.Fatalf("input was not normalized: username=%q hospital=%q", repository.createUsername, repository.createHospital)
	}
	if repository.createHash != "hashed-password" {
		t.Fatalf("repository did not receive a password hash")
	}
	if created.PasswordHash != "" {
		t.Fatal("created staff leaked the password hash")
	}
}

func TestCreateRejectsWeakPassword(t *testing.T) {
	t.Parallel()
	repository := &fakeStaffRepository{}
	service := NewService(repository, fakePasswords{}, fakeTokens{})

	_, err := service.Create(context.Background(), CreateInput{
		Username:     "doctor.one",
		Password:     "short",
		HospitalCode: "hospital-a",
	})
	if !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if repository.createUsername != "" {
		t.Fatal("repository was called for invalid input")
	}
}

func TestLoginReturnsTokenWithoutHash(t *testing.T) {
	t.Parallel()
	expiresAt := time.Now().Add(time.Hour)
	repository := &fakeStaffRepository{found: core.Staff{
		ID:           "staff-1",
		Username:     "doctor.one",
		PasswordHash: "hashed-password",
		HospitalID:   "hospital-id-a",
		HospitalCode: "hospital-a",
	}}
	service := NewService(
		repository,
		fakePasswords{},
		fakeTokens{token: "signed-token", expiresAt: expiresAt},
	)

	result, err := service.Login(context.Background(), LoginInput{
		Username:     " Doctor.One ",
		Password:     "correct-horse-battery",
		HospitalCode: " Hospital-A ",
	})
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	if result.Token != "signed-token" || !result.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected login result: %+v", result)
	}
	if result.Staff.PasswordHash != "" {
		t.Fatal("login result leaked the password hash")
	}
}

func TestLoginHidesCredentialFailureReason(t *testing.T) {
	t.Parallel()
	repository := &fakeStaffRepository{found: core.Staff{PasswordHash: "hash", HospitalCode: "hospital-a"}}
	service := NewService(repository, fakePasswords{compareErr: errors.New("mismatch")}, fakeTokens{})

	_, err := service.Login(context.Background(), LoginInput{
		Username:     "doctor.one",
		Password:     "wrong-password",
		HospitalCode: "hospital-a",
	})
	if !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestLoginRejectsMissingOrMismatchedHospital(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		hospitalInput string
	}{
		{name: "missing hospital"},
		{name: "mismatched hospital", hospitalInput: "hospital-b"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repository := &fakeStaffRepository{found: core.Staff{
				Username:     "doctor.one",
				PasswordHash: "hash",
				HospitalCode: "hospital-a",
			}}
			service := NewService(repository, fakePasswords{}, fakeTokens{token: "must-not-be-issued"})

			result, err := service.Login(context.Background(), LoginInput{
				Username:     "doctor.one",
				Password:     "correct-horse-battery",
				HospitalCode: test.hospitalInput,
			})
			if !errors.Is(err, core.ErrUnauthorized) {
				t.Fatalf("expected unauthorized, got %v", err)
			}
			if result.Token != "" {
				t.Fatalf("unexpected token for invalid hospital: %q", result.Token)
			}
		})
	}
}
