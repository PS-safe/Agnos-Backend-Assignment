package patient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"agnos.dev/hospital-middleware/internal/core"
)

const MaxResults = 100

type Criteria struct {
	NationalID  string
	PassportID  string
	FirstName   string
	MiddleName  string
	LastName    string
	DateOfBirth *time.Time
	PhoneNumber string
	Email       string
}

func (c Criteria) HasAny() bool {
	return c.NationalID != "" || c.PassportID != "" || c.FirstName != "" ||
		c.MiddleName != "" || c.LastName != "" || c.DateOfBirth != nil ||
		c.PhoneNumber != "" || c.Email != ""
}

func (c Criteria) Normalized() Criteria {
	c.NationalID = strings.TrimSpace(c.NationalID)
	c.PassportID = strings.TrimSpace(c.PassportID)
	c.FirstName = strings.TrimSpace(c.FirstName)
	c.MiddleName = strings.TrimSpace(c.MiddleName)
	c.LastName = strings.TrimSpace(c.LastName)
	c.PhoneNumber = strings.TrimSpace(c.PhoneNumber)
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	return c
}

func (c Criteria) Validate() error {
	if utf8.RuneCountInString(c.NationalID) > 64 || utf8.RuneCountInString(c.PassportID) > 64 || utf8.RuneCountInString(c.PhoneNumber) > 64 {
		return fmt.Errorf("%w: identifiers and phone_number must not exceed 64 characters", core.ErrInvalidInput)
	}
	if utf8.RuneCountInString(c.FirstName) > 255 || utf8.RuneCountInString(c.MiddleName) > 255 || utf8.RuneCountInString(c.LastName) > 255 {
		return fmt.Errorf("%w: name fields must not exceed 255 characters", core.ErrInvalidInput)
	}
	if utf8.RuneCountInString(c.Email) > 320 {
		return fmt.Errorf("%w: email must not exceed 320 characters", core.ErrInvalidInput)
	}
	return nil
}

type Repository interface {
	Upsert(ctx context.Context, patient core.Patient) error
	Search(ctx context.Context, hospitalID string, criteria Criteria, limit int) ([]core.Patient, error)
}

type HospitalClient interface {
	SearchByID(ctx context.Context, identifier string) (core.Patient, error)
}

type Service struct {
	repository Repository
	clients    map[string]HospitalClient
}

func NewService(repository Repository, clients map[string]HospitalClient) *Service {
	copyOfClients := make(map[string]HospitalClient, len(clients))
	for code, client := range clients {
		copyOfClients[strings.ToLower(strings.TrimSpace(code))] = client
	}
	return &Service{repository: repository, clients: copyOfClients}
}

func (s *Service) Search(ctx context.Context, auth core.AuthContext, criteria Criteria) ([]core.Patient, error) {
	criteria = criteria.Normalized()
	if auth.HospitalID == "" || auth.HospitalCode == "" {
		return nil, core.ErrUnauthorized
	}
	if !criteria.HasAny() {
		return nil, fmt.Errorf("%w: provide at least one search field", core.ErrInvalidInput)
	}
	if err := criteria.Validate(); err != nil {
		return nil, err
	}

	identifier := criteria.NationalID
	if identifier == "" {
		identifier = criteria.PassportID
	}

	client := s.clients[strings.ToLower(auth.HospitalCode)]
	if identifier != "" && client != nil {
		upstreamPatient, err := client.SearchByID(ctx, identifier)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return []core.Patient{}, nil
			}
			return nil, err
		}
		upstreamPatient.HospitalID = auth.HospitalID
		if err := s.repository.Upsert(ctx, upstreamPatient); err != nil {
			return nil, err
		}
	}

	patients, err := s.repository.Search(ctx, auth.HospitalID, criteria, MaxResults)
	if err != nil {
		return nil, err
	}
	return patients, nil
}
