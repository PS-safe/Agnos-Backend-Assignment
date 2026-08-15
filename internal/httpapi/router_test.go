package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
	"agnos.dev/hospital-middleware/internal/patient"
	"agnos.dev/hospital-middleware/internal/staff"
)

type fakeStaffUseCase struct {
	create func(context.Context, staff.CreateInput) (core.Staff, error)
	login  func(context.Context, staff.LoginInput) (staff.LoginResult, error)
}

func (f fakeStaffUseCase) Create(ctx context.Context, input staff.CreateInput) (core.Staff, error) {
	return f.create(ctx, input)
}

func (f fakeStaffUseCase) Login(ctx context.Context, input staff.LoginInput) (staff.LoginResult, error) {
	return f.login(ctx, input)
}

type fakePatientUseCase struct {
	search func(context.Context, core.AuthContext, patient.Criteria) ([]core.Patient, error)
}

func (f fakePatientUseCase) Search(ctx context.Context, auth core.AuthContext, criteria patient.Criteria) ([]core.Patient, error) {
	return f.search(ctx, auth, criteria)
}

type fakeTokenVerifier struct {
	verify func(string) (core.AuthContext, error)
}

func (f fakeTokenVerifier) Verify(raw string) (core.AuthContext, error) {
	return f.verify(raw)
}

func TestCreateStaffAPI(t *testing.T) {
	t.Parallel()

	t.Run("creates a staff member", func(t *testing.T) {
		t.Parallel()
		createdAt := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)
		staffUseCase := fakeStaffUseCase{
			create: func(_ context.Context, input staff.CreateInput) (core.Staff, error) {
				if input.Username != "doctor.one" || input.Password != "correct-horse-battery" || input.HospitalCode != "hospital-a" {
					t.Fatalf("unexpected create input: %+v", input)
				}
				return core.Staff{
					ID:           "staff-1",
					Username:     "doctor.one",
					HospitalCode: "hospital-a",
					CreatedAt:    createdAt,
				}, nil
			},
			login: unexpectedLogin(t),
		}
		router := testRouter(staffUseCase, unexpectedPatientSearch(t), validTokenVerifier())

		response := performRequest(router, http.MethodPost, "/staff/create", `{
			"username":"doctor.one",
			"password":"correct-horse-battery",
			"hospital":"hospital-a"
		}`, "")

		if response.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "password") {
			t.Fatalf("response leaked a password field: %s", response.Body.String())
		}
		var body struct {
			Data struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Hospital string `json:"hospital"`
			} `json:"data"`
		}
		decodeResponse(t, response, &body)
		if body.Data.ID != "staff-1" || body.Data.Username != "doctor.one" || body.Data.Hospital != "hospital-a" {
			t.Fatalf("unexpected response: %+v", body.Data)
		}
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		t.Parallel()
		staffUseCase := fakeStaffUseCase{
			create: func(context.Context, staff.CreateInput) (core.Staff, error) {
				return core.Staff{}, errors.Join(core.ErrInvalidInput, errors.New("password is too short"))
			},
			login: unexpectedLogin(t),
		}
		router := testRouter(staffUseCase, unexpectedPatientSearch(t), validTokenVerifier())

		response := performRequest(router, http.MethodPost, "/staff/create", `{
			"username":"doctor.one",
			"password":"short",
			"hospital":"hospital-a"
		}`, "")

		assertErrorResponse(t, response, http.StatusBadRequest, "invalid_request")
	})

	t.Run("rejects unknown JSON fields", func(t *testing.T) {
		t.Parallel()
		staffUseCase := fakeStaffUseCase{create: unexpectedCreate(t), login: unexpectedLogin(t)}
		router := testRouter(staffUseCase, unexpectedPatientSearch(t), validTokenVerifier())

		response := performRequest(router, http.MethodPost, "/staff/create", `{
			"username":"doctor.one",
			"password":"correct-horse-battery",
			"hospital":"hospital-a",
			"admin":true
		}`, "")

		assertErrorResponse(t, response, http.StatusBadRequest, "invalid_request")
	})
}

func TestLoginStaffAPI(t *testing.T) {
	t.Parallel()

	t.Run("returns a bearer token", func(t *testing.T) {
		t.Parallel()
		staffUseCase := fakeStaffUseCase{
			create: unexpectedCreate(t),
			login: func(_ context.Context, input staff.LoginInput) (staff.LoginResult, error) {
				if input.Username != "doctor.one" || input.Password != "correct-horse-battery" {
					t.Fatalf("unexpected login input: %+v", input)
				}
				return staff.LoginResult{
					Staff: core.Staff{
						ID:           "staff-1",
						Username:     "doctor.one",
						HospitalCode: "hospital-a",
						CreatedAt:    time.Now().UTC(),
					},
					Token:     "signed-token",
					ExpiresAt: time.Now().Add(time.Hour),
				}, nil
			},
		}
		router := testRouter(staffUseCase, unexpectedPatientSearch(t), validTokenVerifier())

		response := performRequest(router, http.MethodPost, "/staff/login", `{
			"username":"doctor.one",
			"password":"correct-horse-battery"
		}`, "")

		if response.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
		}
		var body struct {
			Data struct {
				AccessToken string `json:"access_token"`
				TokenType   string `json:"token_type"`
			} `json:"data"`
		}
		decodeResponse(t, response, &body)
		if body.Data.AccessToken != "signed-token" || body.Data.TokenType != "Bearer" {
			t.Fatalf("unexpected token response: %+v", body.Data)
		}
	})

	t.Run("rejects incorrect credentials", func(t *testing.T) {
		t.Parallel()
		staffUseCase := fakeStaffUseCase{
			create: unexpectedCreate(t),
			login: func(context.Context, staff.LoginInput) (staff.LoginResult, error) {
				return staff.LoginResult{}, core.ErrUnauthorized
			},
		}
		router := testRouter(staffUseCase, unexpectedPatientSearch(t), validTokenVerifier())

		response := performRequest(router, http.MethodPost, "/staff/login", `{
			"username":"doctor.one",
			"password":"incorrect-password"
		}`, "")

		assertErrorResponse(t, response, http.StatusUnauthorized, "unauthorized")
	})
}

func TestSearchPatientsAPI(t *testing.T) {
	t.Parallel()

	t.Run("returns patients from the authenticated hospital", func(t *testing.T) {
		t.Parallel()
		patientUseCase := fakePatientUseCase{
			search: func(_ context.Context, auth core.AuthContext, criteria patient.Criteria) ([]core.Patient, error) {
				if auth.HospitalID != "hospital-id-a" || auth.HospitalCode != "hospital-a" {
					t.Fatalf("unexpected auth context: %+v", auth)
				}
				if criteria.NationalID != "1234567890123" || criteria.FirstName != "Ada" {
					t.Fatalf("unexpected criteria: %+v", criteria)
				}
				return []core.Patient{{
					HospitalID:  "hospital-id-a",
					FirstNameEN: "Ada",
					LastNameEN:  "Lovelace",
					DateOfBirth: time.Date(1815, time.December, 10, 0, 0, 0, 0, time.UTC),
					PatientHN:   "HN-001",
					NationalID:  "1234567890123",
					Gender:      "F",
				}}, nil
			},
		}
		router := testRouter(
			fakeStaffUseCase{create: unexpectedCreate(t), login: unexpectedLogin(t)},
			patientUseCase,
			validTokenVerifier(),
		)

		response := performRequest(router, http.MethodGet, "/patient/search?national_id=1234567890123&first_name=Ada", "", "Bearer valid-token")

		if response.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
		}
		var body struct {
			Data struct {
				Count    int               `json:"count"`
				Patients []patientResponse `json:"patients"`
			} `json:"data"`
		}
		decodeResponse(t, response, &body)
		if body.Data.Count != 1 || len(body.Data.Patients) != 1 || body.Data.Patients[0].PatientHN != "HN-001" {
			t.Fatalf("unexpected patient response: %+v", body.Data)
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		t.Parallel()
		router := testRouter(
			fakeStaffUseCase{create: unexpectedCreate(t), login: unexpectedLogin(t)},
			unexpectedPatientSearch(t),
			validTokenVerifier(),
		)

		response := performRequest(router, http.MethodGet, "/patient/search?national_id=1234567890123", "", "")

		assertErrorResponse(t, response, http.StatusUnauthorized, "unauthorized")
	})

	t.Run("rejects a malformed date", func(t *testing.T) {
		t.Parallel()
		router := testRouter(
			fakeStaffUseCase{create: unexpectedCreate(t), login: unexpectedLogin(t)},
			unexpectedPatientSearch(t),
			validTokenVerifier(),
		)

		response := performRequest(router, http.MethodGet, "/patient/search?date_of_birth=10-12-1815", "", "Bearer valid-token")

		assertErrorResponse(t, response, http.StatusBadRequest, "invalid_request")
	})

	t.Run("rejects an empty search", func(t *testing.T) {
		t.Parallel()
		patientUseCase := fakePatientUseCase{
			search: func(context.Context, core.AuthContext, patient.Criteria) ([]core.Patient, error) {
				return nil, errors.Join(core.ErrInvalidInput, errors.New("provide at least one search field"))
			},
		}
		router := testRouter(
			fakeStaffUseCase{create: unexpectedCreate(t), login: unexpectedLogin(t)},
			patientUseCase,
			validTokenVerifier(),
		)

		response := performRequest(router, http.MethodGet, "/patient/search", "", "Bearer valid-token")

		assertErrorResponse(t, response, http.StatusBadRequest, "invalid_request")
	})

	t.Run("maps a Hospital HIS failure to bad gateway", func(t *testing.T) {
		t.Parallel()
		patientUseCase := fakePatientUseCase{
			search: func(context.Context, core.AuthContext, patient.Criteria) ([]core.Patient, error) {
				return nil, core.ErrUpstream
			},
		}
		router := testRouter(
			fakeStaffUseCase{create: unexpectedCreate(t), login: unexpectedLogin(t)},
			patientUseCase,
			validTokenVerifier(),
		)

		response := performRequest(router, http.MethodGet, "/patient/search?national_id=1234567890123", "", "Bearer valid-token")

		assertErrorResponse(t, response, http.StatusBadGateway, "hospital_unavailable")
	})
}

func TestHealthAndUnknownRoute(t *testing.T) {
	t.Parallel()
	router := testRouter(
		fakeStaffUseCase{create: unexpectedCreate(t), login: unexpectedLogin(t)},
		unexpectedPatientSearch(t),
		validTokenVerifier(),
	)

	health := performRequest(router, http.MethodGet, "/health", "", "")
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected health response: %d %s", health.Code, health.Body.String())
	}

	missing := performRequest(router, http.MethodGet, "/missing", "", "")
	assertErrorResponse(t, missing, http.StatusNotFound, "route_not_found")
}

func TestApplicationErrorMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "unknown hospital", err: errors.Join(core.ErrNotFound, errors.New("hospital does not exist")), status: http.StatusNotFound, code: "not_found"},
		{name: "duplicate username", err: errors.Join(core.ErrConflict, errors.New("username already exists")), status: http.StatusConflict, code: "conflict"},
		{name: "internal error", err: core.ErrInternal, status: http.StatusInternalServerError, code: "internal_error"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			staffUseCase := fakeStaffUseCase{
				create: func(context.Context, staff.CreateInput) (core.Staff, error) {
					return core.Staff{}, test.err
				},
				login: unexpectedLogin(t),
			}
			router := testRouter(staffUseCase, unexpectedPatientSearch(t), validTokenVerifier())
			response := performRequest(router, http.MethodPost, "/staff/create", `{
				"username":"doctor.one",
				"password":"correct-horse-battery",
				"hospital":"hospital-a"
			}`, "")
			assertErrorResponse(t, response, test.status, test.code)
		})
	}
}

func testRouter(staffUseCase StaffUseCase, patientUseCase PatientUseCase, verifier TokenVerifier) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(Dependencies{
		Staff:    staffUseCase,
		Patients: patientUseCase,
		Tokens:   verifier,
		Logger:   logger,
	})
}

func validTokenVerifier() fakeTokenVerifier {
	return fakeTokenVerifier{verify: func(raw string) (core.AuthContext, error) {
		if raw != "valid-token" {
			return core.AuthContext{}, core.ErrUnauthorized
		}
		return core.AuthContext{
			StaffID:      "staff-1",
			HospitalID:   "hospital-id-a",
			HospitalCode: "hospital-a",
			Username:     "doctor.one",
		}, nil
	}}
}

func performRequest(router http.Handler, method, target, body, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
}

func assertErrorResponse(t *testing.T, response *httptest.ResponseRecorder, expectedStatus int, expectedCode string) {
	t.Helper()
	if response.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d: %s", expectedStatus, response.Code, response.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeResponse(t, response, &body)
	if body.Error.Code != expectedCode {
		t.Fatalf("expected error code %q, got %q: %s", expectedCode, body.Error.Code, response.Body.String())
	}
}

func unexpectedCreate(t *testing.T) func(context.Context, staff.CreateInput) (core.Staff, error) {
	t.Helper()
	return func(context.Context, staff.CreateInput) (core.Staff, error) {
		t.Fatal("unexpected create call")
		return core.Staff{}, nil
	}
}

func unexpectedLogin(t *testing.T) func(context.Context, staff.LoginInput) (staff.LoginResult, error) {
	t.Helper()
	return func(context.Context, staff.LoginInput) (staff.LoginResult, error) {
		t.Fatal("unexpected login call")
		return staff.LoginResult{}, nil
	}
}

func unexpectedPatientSearch(t *testing.T) fakePatientUseCase {
	t.Helper()
	return fakePatientUseCase{search: func(context.Context, core.AuthContext, patient.Criteria) ([]core.Patient, error) {
		t.Fatal("unexpected patient search call")
		return nil, nil
	}}
}
