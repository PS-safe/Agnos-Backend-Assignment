package hospital

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
)

const maxHospitalResponseBytes = 1 << 20

type HospitalAClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type hospitalAPatient struct {
	FirstNameTH  string `json:"first_name_th"`
	MiddleNameTH string `json:"middle_name_th"`
	LastNameTH   string `json:"last_name_th"`
	FirstNameEN  string `json:"first_name_en"`
	MiddleNameEN string `json:"middle_name_en"`
	LastNameEN   string `json:"last_name_en"`
	DateOfBirth  string `json:"date_of_birth"`
	PatientHN    string `json:"patient_hn"`
	NationalID   string `json:"national_id"`
	PassportID   string `json:"passport_id"`
	PhoneNumber  string `json:"phone_number"`
	Email        string `json:"email"`
	Gender       string `json:"gender"`
}

func NewHospitalAClient(baseURL, apiKey string, httpClient *http.Client) (*HospitalAClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid Hospital A base URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("Hospital A base URL must not contain a query or fragment")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}

	return &HospitalAClient{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: httpClient,
	}, nil
}

func (c *HospitalAClient) SearchByID(ctx context.Context, identifier string) (core.Patient, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return core.Patient{}, fmt.Errorf("%w: patient identifier is required", core.ErrInvalidInput)
	}

	endpoint := c.baseURL + "/patient/search/" + url.PathEscape(identifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return core.Patient{}, fmt.Errorf("build Hospital A request: %w", core.ErrInternal)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return core.Patient{}, fmt.Errorf("%w: Hospital A request failed", core.ErrUpstream)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return core.Patient{}, core.ErrNotFound
	case resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return core.Patient{}, fmt.Errorf("%w: Hospital A returned status %d", core.ErrUpstream, resp.StatusCode)
	}

	var payload hospitalAPatient
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxHospitalResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return core.Patient{}, fmt.Errorf("%w: invalid Hospital A response", core.ErrUpstream)
	}

	dateOfBirth, err := time.Parse("2006-01-02", payload.DateOfBirth)
	if err != nil {
		return core.Patient{}, fmt.Errorf("%w: invalid Hospital A date_of_birth", core.ErrUpstream)
	}
	gender := strings.ToUpper(strings.TrimSpace(payload.Gender))
	if gender != "M" && gender != "F" {
		return core.Patient{}, fmt.Errorf("%w: invalid Hospital A gender", core.ErrUpstream)
	}
	if strings.TrimSpace(payload.PatientHN) == "" {
		return core.Patient{}, fmt.Errorf("%w: Hospital A patient_hn is missing", core.ErrUpstream)
	}
	if strings.TrimSpace(payload.NationalID) == "" && strings.TrimSpace(payload.PassportID) == "" {
		return core.Patient{}, fmt.Errorf("%w: Hospital A patient identifier is missing", core.ErrUpstream)
	}

	return core.Patient{
		FirstNameTH:   strings.TrimSpace(payload.FirstNameTH),
		MiddleNameTH:  strings.TrimSpace(payload.MiddleNameTH),
		LastNameTH:    strings.TrimSpace(payload.LastNameTH),
		FirstNameEN:   strings.TrimSpace(payload.FirstNameEN),
		MiddleNameEN:  strings.TrimSpace(payload.MiddleNameEN),
		LastNameEN:    strings.TrimSpace(payload.LastNameEN),
		DateOfBirth:   dateOfBirth,
		PatientHN:     strings.TrimSpace(payload.PatientHN),
		NationalID:    strings.TrimSpace(payload.NationalID),
		PassportID:    strings.TrimSpace(payload.PassportID),
		PhoneNumber:   strings.TrimSpace(payload.PhoneNumber),
		Email:         strings.ToLower(strings.TrimSpace(payload.Email)),
		Gender:        gender,
		SourceSystem:  "hospital-a",
		SourceUpdated: time.Now().UTC(),
	}, nil
}
