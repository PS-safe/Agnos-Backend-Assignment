package patient

import (
	"context"
	"errors"
	"testing"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
)

type fakeRepository struct {
	upserted    []core.Patient
	searchAuth  string
	searchInput Criteria
	searchLimit int
	results     []core.Patient
	err         error
}

func (f *fakeRepository) Upsert(_ context.Context, value core.Patient) error {
	f.upserted = append(f.upserted, value)
	return f.err
}

func (f *fakeRepository) Search(_ context.Context, hospitalID string, criteria Criteria, limit int) ([]core.Patient, error) {
	f.searchAuth = hospitalID
	f.searchInput = criteria
	f.searchLimit = limit
	return f.results, f.err
}

type fakeHospitalClient struct {
	identifiers []string
	result      core.Patient
	err         error
}

func (f *fakeHospitalClient) SearchByID(_ context.Context, identifier string) (core.Patient, error) {
	f.identifiers = append(f.identifiers, identifier)
	return f.result, f.err
}

func TestSearchUsesHospitalContextAndHydratesCache(t *testing.T) {
	t.Parallel()
	dob := time.Date(1990, time.January, 2, 0, 0, 0, 0, time.UTC)
	repository := &fakeRepository{results: []core.Patient{{PatientHN: "HN-1"}}}
	client := &fakeHospitalClient{result: core.Patient{
		PatientHN:   "HN-1",
		NationalID:  "1234567890123",
		DateOfBirth: dob,
	}}
	service := NewService(repository, map[string]HospitalClient{"hospital-a": client})
	auth := core.AuthContext{HospitalID: "hospital-id-a", HospitalCode: "hospital-a"}

	results, err := service.Search(context.Background(), auth, Criteria{NationalID: " 1234567890123 "})
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if len(client.identifiers) != 1 || client.identifiers[0] != "1234567890123" {
		t.Fatalf("unexpected upstream identifiers: %v", client.identifiers)
	}
	if len(repository.upserted) != 1 || repository.upserted[0].HospitalID != "hospital-id-a" {
		t.Fatalf("upsert did not receive the authenticated hospital: %+v", repository.upserted)
	}
	if repository.searchAuth != "hospital-id-a" || repository.searchLimit != MaxResults {
		t.Fatalf("repository search was not scoped: hospital=%q limit=%d", repository.searchAuth, repository.searchLimit)
	}
	if len(results) != 1 || results[0].PatientHN != "HN-1" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSearchByNameUsesNormalizedCacheOnly(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{}
	client := &fakeHospitalClient{}
	service := NewService(repository, map[string]HospitalClient{"hospital-a": client})
	auth := core.AuthContext{HospitalID: "hospital-id-a", HospitalCode: "hospital-a"}

	_, err := service.Search(context.Background(), auth, Criteria{FirstName: "  Ada  "})
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if len(client.identifiers) != 0 {
		t.Fatalf("name-only search unexpectedly called upstream: %v", client.identifiers)
	}
	if repository.searchInput.FirstName != "Ada" {
		t.Fatalf("criteria were not normalized: %+v", repository.searchInput)
	}
}

func TestSearchRejectsEmptyCriteria(t *testing.T) {
	t.Parallel()
	service := NewService(&fakeRepository{}, nil)
	auth := core.AuthContext{HospitalID: "hospital-id-a", HospitalCode: "hospital-a"}

	_, err := service.Search(context.Background(), auth, Criteria{})
	if !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestSearchRejectsOversizedCriteria(t *testing.T) {
	t.Parallel()
	service := NewService(&fakeRepository{}, nil)
	auth := core.AuthContext{HospitalID: "hospital-id-a", HospitalCode: "hospital-a"}

	_, err := service.Search(context.Background(), auth, Criteria{NationalID: string(make([]byte, 65))})
	if !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestSearchRejectsMissingHospitalContext(t *testing.T) {
	t.Parallel()
	service := NewService(&fakeRepository{}, nil)
	_, err := service.Search(context.Background(), core.AuthContext{}, Criteria{NationalID: "123"})
	if !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestSearchDoesNotUseCallerSuppliedHospital(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{}
	service := NewService(repository, nil)
	auth := core.AuthContext{HospitalID: "hospital-id-a", HospitalCode: "hospital-a"}

	_, err := service.Search(context.Background(), auth, Criteria{Email: "patient@example.com"})
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if repository.searchAuth != "hospital-id-a" {
		t.Fatalf("expected hospital-id-a scope, got %q", repository.searchAuth)
	}
}

func TestSearchMapsUpstreamNotFoundToEmptyResult(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{results: []core.Patient{{PatientHN: "stale"}}}
	client := &fakeHospitalClient{err: core.ErrNotFound}
	service := NewService(repository, map[string]HospitalClient{"hospital-a": client})
	auth := core.AuthContext{HospitalID: "hospital-id-a", HospitalCode: "hospital-a"}

	results, err := service.Search(context.Background(), auth, Criteria{PassportID: "P123"})
	if err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %+v", results)
	}
	if repository.searchAuth != "" {
		t.Fatal("repository should not be consulted after an authoritative upstream 404")
	}
}

func TestSearchPropagatesUpstreamAndRepositoryFailures(t *testing.T) {
	t.Parallel()
	auth := core.AuthContext{HospitalID: "hospital-id-a", HospitalCode: "hospital-a"}

	t.Run("upstream failure", func(t *testing.T) {
		t.Parallel()
		client := &fakeHospitalClient{err: core.ErrUpstream}
		service := NewService(&fakeRepository{}, map[string]HospitalClient{"hospital-a": client})
		_, err := service.Search(context.Background(), auth, Criteria{NationalID: "123"})
		if !errors.Is(err, core.ErrUpstream) {
			t.Fatalf("expected upstream error, got %v", err)
		}
	})

	t.Run("cache failure", func(t *testing.T) {
		t.Parallel()
		repository := &fakeRepository{err: core.ErrInternal}
		client := &fakeHospitalClient{result: core.Patient{PatientHN: "HN-1"}}
		service := NewService(repository, map[string]HospitalClient{"hospital-a": client})
		_, err := service.Search(context.Background(), auth, Criteria{NationalID: "123"})
		if !errors.Is(err, core.ErrInternal) {
			t.Fatalf("expected internal error, got %v", err)
		}
	})
}
