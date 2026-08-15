package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadValidConfigurationAndDefaults(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("JWT_EXPIRY", "90m")
	t.Setenv("HOSPITAL_HTTP_TIMEOUT", "7")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPAddress != ":8080" || cfg.JWTIssuer != "agnos-hospital-middleware" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.JWTExpiry != 90*time.Minute || cfg.HospitalHTTPTimeout != 7*time.Second {
		t.Fatalf("unexpected durations: expiry=%s timeout=%s", cfg.JWTExpiry, cfg.HospitalHTTPTimeout)
	}
}

func TestLoadReportsAllRequiredConfigurationErrors(t *testing.T) {
	setBaseEnvironment(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "short")

	_, err := Load()
	if err == nil {
		t.Fatal("expected configuration error")
	}
	message := err.Error()
	for _, expected := range []string{"DATABASE_URL", "JWT_SECRET"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("expected %s error in %q", expected, message)
		}
	}
}

func TestLoadRejectsInvalidDurations(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "negative seconds", key: "JWT_EXPIRY", value: "-1"},
		{name: "invalid duration", key: "HOSPITAL_HTTP_TIMEOUT", value: "later"},
		{name: "zero duration", key: "JWT_EXPIRY", value: "0s"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setBaseEnvironment(t)
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatalf("expected %s=%q to fail", test.key, test.value)
			}
		})
	}
}

func setBaseEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("HTTP_ADDRESS", "")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("JWT_ISSUER", "")
	t.Setenv("JWT_EXPIRY", "")
	t.Setenv("HOSPITAL_A_BASE_URL", "https://hospital-a.example")
	t.Setenv("HOSPITAL_A_API_KEY", "")
	t.Setenv("HOSPITAL_HTTP_TIMEOUT", "")
}
