package staff

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"agnos.dev/hospital-middleware/internal/core"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{3,64}$`)

type Repository interface {
	Create(ctx context.Context, username, passwordHash, hospitalCode string) (core.Staff, error)
	GetByUsername(ctx context.Context, username string) (core.Staff, error)
}

type PasswordManager interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

type TokenIssuer interface {
	Issue(staff core.Staff) (token string, expiresAt time.Time, err error)
}

type Service struct {
	repository Repository
	passwords  PasswordManager
	tokens     TokenIssuer
}

type CreateInput struct {
	Username     string
	Password     string
	HospitalCode string
}

type LoginInput struct {
	Username string
	Password string
}

type LoginResult struct {
	Staff     core.Staff
	Token     string
	ExpiresAt time.Time
}

func NewService(repository Repository, passwords PasswordManager, tokens TokenIssuer) *Service {
	return &Service{repository: repository, passwords: passwords, tokens: tokens}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (core.Staff, error) {
	username := normalizeUsername(input.Username)
	hospitalCode := strings.ToLower(strings.TrimSpace(input.HospitalCode))
	if err := validateCredentials(username, input.Password); err != nil {
		return core.Staff{}, err
	}
	if hospitalCode == "" || len(hospitalCode) > 64 {
		return core.Staff{}, fmt.Errorf("%w: hospital is required", core.ErrInvalidInput)
	}

	hash, err := s.passwords.Hash(input.Password)
	if err != nil {
		return core.Staff{}, fmt.Errorf("hash password: %w", core.ErrInternal)
	}

	created, err := s.repository.Create(ctx, username, hash, hospitalCode)
	if err != nil {
		return core.Staff{}, err
	}
	created.PasswordHash = ""
	return created, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	username := normalizeUsername(input.Username)
	if username == "" || input.Password == "" {
		return LoginResult{}, core.ErrUnauthorized
	}

	found, err := s.repository.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return LoginResult{}, core.ErrUnauthorized
		}
		return LoginResult{}, err
	}
	if err := s.passwords.Compare(found.PasswordHash, input.Password); err != nil {
		return LoginResult{}, core.ErrUnauthorized
	}

	token, expiresAt, err := s.tokens.Issue(found)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token: %w", core.ErrInternal)
	}
	found.PasswordHash = ""
	return LoginResult{Staff: found, Token: token, ExpiresAt: expiresAt}, nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func validateCredentials(username, password string) error {
	if !usernamePattern.MatchString(username) {
		return fmt.Errorf("%w: username must be 3-64 characters and contain only letters, numbers, dot, underscore, or hyphen", core.ErrInvalidInput)
	}
	passwordBytes := len([]byte(password))
	if !utf8.ValidString(password) || passwordBytes < 12 || passwordBytes > 72 {
		return fmt.Errorf("%w: password must be valid UTF-8 and contain 12-72 bytes", core.ErrInvalidInput)
	}
	return nil
}
