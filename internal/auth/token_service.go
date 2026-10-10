package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"backend/internal/config"
	tokenutil "backend/internal/token"
)

const refreshTokenBytes = 32

type AccessTokenClaims struct {
	UserID    string `json:"uid"`
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret          []byte
	ttl             time.Duration
	refreshTokenTTL time.Duration
}

func NewTokenService(cfg config.Config) *TokenService {
	return &TokenService{
		secret:          []byte(cfg.Security.JWTSecret),
		ttl:             cfg.Security.AccessTokenTTL,
		refreshTokenTTL: cfg.Security.RefreshTokenTTL,
	}
}

func (s *TokenService) CreateAccessToken(
	userID string,
	sessionID string,
) (string, error) {
	now := time.Now()

	tokenID, err := tokenutil.Generate(16)
	if err != nil {
		return "", err
	}

	claims := AccessTokenClaims{
		UserID:    userID,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "wraplink-api",
			Audience:  jwt.ClaimStrings{"wraplink-web"},
			Subject:   userID,
			ID:        tokenID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	return token.SignedString(s.secret)
}

func (s *TokenService) ParseAccessToken(
	value string,
) (*AccessTokenClaims, error) {
	token, err := jwt.ParseWithClaims(
		value,
		&AccessTokenClaims{},
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}

			return s.secret, nil
		},
		jwt.WithIssuer("wraplink-api"),
		jwt.WithAudience("wraplink-web"),
	)

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*AccessTokenClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}

func (s *TokenService) CreateRefreshToken() (
	string,
	time.Time,
	error,
) {
	value, err := tokenutil.Generate(refreshTokenBytes)
	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(s.refreshTokenTTL)

	return value, expiresAt, nil
}
