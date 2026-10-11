package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	TypeAccess  = "access"
	TypeRefresh = "refresh"
)

var ErrInvalidToken = errors.New("invalid token")

type claims struct {
	Type string `json:"typ"`
	jwt.RegisteredClaims
}

// TokenManager issues and verifies access / refresh JWTs (HS256).
type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenManager(secret string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (m *TokenManager) NewAccess(userID int64) (string, error) {
	return m.sign(userID, TypeAccess, m.accessTTL)
}

func (m *TokenManager) NewRefresh(userID int64) (string, error) {
	return m.sign(userID, TypeRefresh, m.refreshTTL)
}

// AccessTTL is exposed so handlers can tell clients when the token expires.
func (m *TokenManager) AccessTTL() time.Duration { return m.accessTTL }

func (m *TokenManager) sign(userID int64, typ string, ttl time.Duration) (string, error) {
	now := time.Now()
	c := claims{
		Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
}

// Parse validates the token signature, expiry and expected type, and returns the user id.
func (m *TokenManager) Parse(token, expectedType string) (int64, error) {
	var c claims
	t, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !t.Valid || c.Type != expectedType {
		return 0, ErrInvalidToken
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return id, nil
}
