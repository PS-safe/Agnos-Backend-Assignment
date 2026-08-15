package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agnos.dev/hospital-middleware/internal/config"
	"agnos.dev/hospital-middleware/internal/hospital"
	"agnos.dev/hospital-middleware/internal/httpapi"
	"agnos.dev/hospital-middleware/internal/patient"
	"agnos.dev/hospital-middleware/internal/security"
	"agnos.dev/hospital-middleware/internal/staff"
	"agnos.dev/hospital-middleware/internal/storage/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	startupContext, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startupCancel()

	pool, err := postgres.NewPool(startupContext, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database pool setup failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(startupContext); err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}

	tokenManager := security.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTExpiry)
	staffService := staff.NewService(
		postgres.NewStaffRepository(pool),
		security.PasswordManager{},
		tokenManager,
	)

	hospitalAClient, err := hospital.NewHospitalAClient(
		cfg.HospitalABaseURL,
		cfg.HospitalAAPIKey,
		&http.Client{Timeout: cfg.HospitalHTTPTimeout},
	)
	if err != nil {
		logger.Error("Hospital A client setup failed", "error", err)
		os.Exit(1)
	}
	patientService := patient.NewService(
		postgres.NewPatientRepository(pool),
		map[string]patient.HospitalClient{"hospital-a": hospitalAClient},
	)

	router := httpapi.NewRouter(httpapi.Dependencies{
		Staff:    staffService,
		Patients: patientService,
		Tokens:   tokenManager,
		Logger:   logger,
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		logger.Info("HTTP server started", "address", cfg.HTTPAddress)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			stop()
		}
	}()

	<-shutdownSignal.Done()
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("HTTP shutdown failed", "error", err)
		return
	}
	logger.Info("HTTP server stopped")
}
