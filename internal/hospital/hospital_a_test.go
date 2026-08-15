package hospital

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
)

func TestHospitalAClientSearchByID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/patient/search/1234567890123" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("missing API key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"first_name_th":"เอดา",
			"middle_name_th":"",
			"last_name_th":"เลิฟเลซ",
			"first_name_en":"Ada",
			"middle_name_en":"",
			"last_name_en":"Lovelace",
			"date_of_birth":"1815-12-10",
			"patient_hn":"HN-001",
			"national_id":"1234567890123",
			"passport_id":"",
			"phone_number":"0800000000",
			"email":"ADA@EXAMPLE.COM",
			"gender":"f"
		}`))
	}))
	defer server.Close()

	client, err := NewHospitalAClient(server.URL, "test-key", &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	result, err := client.SearchByID(t.Context(), "1234567890123")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.FirstNameEN != "Ada" || result.PatientHN != "HN-001" || result.Gender != "F" {
		t.Fatalf("unexpected patient: %+v", result)
	}
	if result.Email != "ada@example.com" || result.SourceSystem != "hospital-a" {
		t.Fatalf("patient was not normalized: %+v", result)
	}
}

func TestHospitalAClientErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		body        string
		expectedErr error
	}{
		{name: "not found", status: http.StatusNotFound, body: `{}`, expectedErr: core.ErrNotFound},
		{name: "server error", status: http.StatusServiceUnavailable, body: `{}`, expectedErr: core.ErrUpstream},
		{name: "invalid date", status: http.StatusOK, body: `{"date_of_birth":"not-a-date","patient_hn":"HN-1","gender":"F"}`, expectedErr: core.ErrUpstream},
		{name: "invalid gender", status: http.StatusOK, body: `{"date_of_birth":"2000-01-01","patient_hn":"HN-1","national_id":"123","gender":"X"}`, expectedErr: core.ErrUpstream},
		{name: "missing identifier", status: http.StatusOK, body: `{"date_of_birth":"2000-01-01","patient_hn":"HN-1","gender":"F"}`, expectedErr: core.ErrUpstream},
		{name: "invalid JSON", status: http.StatusOK, body: `{`, expectedErr: core.ErrUpstream},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			client, err := NewHospitalAClient(server.URL, "", &http.Client{Timeout: time.Second})
			if err != nil {
				t.Fatalf("create client: %v", err)
			}
			_, err = client.SearchByID(t.Context(), "ID-1")
			if !errors.Is(err, test.expectedErr) {
				t.Fatalf("expected %v, got %v", test.expectedErr, err)
			}
		})
	}
}

func TestHospitalAClientRejectsInvalidConfigurationAndInput(t *testing.T) {
	t.Parallel()
	if _, err := NewHospitalAClient("not-a-url", "", nil); err == nil {
		t.Fatal("expected invalid base URL to fail")
	}
	client, err := NewHospitalAClient("https://hospital-a.example", "", nil)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := client.SearchByID(t.Context(), " "); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}
