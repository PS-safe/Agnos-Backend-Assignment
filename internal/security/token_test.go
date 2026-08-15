package security

import (
	"errors"
	"strings"
	"testing"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
)

func TestTokenRoundTripAndTamperDetection(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	manager := NewTokenManager(strings.Repeat("s", 32), "test-issuer", time.Hour)
	manager.now = func() time.Time { return now }
	staff := core.Staff{
		ID:           "staff-1",
		HospitalID:   "hospital-id-a",
		HospitalCode: "hospital-a",
		Username:     "doctor.one",
	}

	raw, expiresAt, err := manager.Issue(staff)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if !expiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("unexpected expiration: %s", expiresAt)
	}
	auth, err := manager.Verify(raw)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if auth.StaffID != staff.ID || auth.HospitalID != staff.HospitalID || auth.HospitalCode != staff.HospitalCode {
		t.Fatalf("unexpected claims: %+v", auth)
	}

	tampered := raw[:len(raw)-1] + "x"
	_, err = manager.Verify(tampered)
	if !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expected unauthorized for tampered token, got %v", err)
	}
}
