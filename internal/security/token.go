package security

import (
	"fmt"
	"time"

	"agnos.dev/hospital-middleware/internal/core"
	"github.com/golang-jwt/jwt/v5"
)

type TokenManager struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

type Claims struct {
	HospitalID   string `json:"hospital_id"`
	HospitalCode string `json:"hospital_code"`
	Username     string `json:"username"`
	jwt.RegisteredClaims
}

func NewTokenManager(secret, issuer string, ttl time.Duration) *TokenManager {
	return &TokenManager{
		secret: []byte(secret),
		issuer: issuer,
		ttl:    ttl,
		now:    time.Now,
	}
}

func (m *TokenManager) Issue(staff core.Staff) (string, time.Time, error) {
	now := m.now().UTC()
	expiresAt := now.Add(m.ttl)
	claims := Claims{
		HospitalID:   staff.HospitalID,
		HospitalCode: staff.HospitalCode,
		Username:     staff.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   staff.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, expiresAt, nil
}

func (m *TokenManager) Verify(raw string) (core.AuthContext, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		raw,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
			}
			return m.secret, nil
		},
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil || !token.Valid {
		return core.AuthContext{}, core.ErrUnauthorized
	}
	if claims.Subject == "" || claims.HospitalID == "" || claims.HospitalCode == "" || claims.Username == "" {
		return core.AuthContext{}, core.ErrUnauthorized
	}

	return core.AuthContext{
		StaffID:      claims.Subject,
		HospitalID:   claims.HospitalID,
		HospitalCode: claims.HospitalCode,
		Username:     claims.Username,
	}, nil
}
