package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddress         string
	DatabaseURL         string
	JWTSecret           string
	JWTIssuer           string
	JWTExpiry           time.Duration
	HospitalABaseURL    string
	HospitalAAPIKey     string
	HospitalHTTPTimeout time.Duration
}

func Load() (Config, error) {
	jwtExpiry, err := durationEnv("JWT_EXPIRY", 8*time.Hour)
	if err != nil {
		return Config{}, err
	}

	hospitalTimeout, err := durationEnv("HOSPITAL_HTTP_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddress:         stringEnv("HTTP_ADDRESS", ":8080"),
		DatabaseURL:         strings.TrimSpace(os.Getenv("DATABASE_URL")),
		JWTSecret:           os.Getenv("JWT_SECRET"),
		JWTIssuer:           stringEnv("JWT_ISSUER", "agnos-hospital-middleware"),
		JWTExpiry:           jwtExpiry,
		HospitalABaseURL:    stringEnv("HOSPITAL_A_BASE_URL", "https://hospital-a.api.co.th"),
		HospitalAAPIKey:     strings.TrimSpace(os.Getenv("HOSPITAL_A_API_KEY")),
		HospitalHTTPTimeout: hospitalTimeout,
	}

	var validationErrors []error
	if cfg.DatabaseURL == "" {
		validationErrors = append(validationErrors, errors.New("DATABASE_URL is required"))
	}
	if len(cfg.JWTSecret) < 32 {
		validationErrors = append(validationErrors, errors.New("JWT_SECRET must contain at least 32 characters"))
	}
	if len(validationErrors) > 0 {
		return Config{}, errors.Join(validationErrors...)
	}

	return cfg, nil
}

func stringEnv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds <= 0 {
			return 0, fmt.Errorf("%s must be positive", name)
		}
		return time.Duration(seconds) * time.Second, nil
	}

	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration or number of seconds", name)
	}
	return duration, nil
}
