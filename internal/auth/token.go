package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"backend/internal/config"
	randomtoken "backend/internal/token"
)

type AccessTokenClaims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenService(
	cfg config.Config,
) *TokenService {
	return &TokenService{
		secret: []byte(cfg.Security.JWTSecret),
		ttl:    cfg.Security.AccessTokenTTL,
	}
}

func (s *TokenService) CreateAccessToken(
	userID string,
) (string, error) {

	now := time.Now()

	claims := AccessTokenClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(now),

			ExpiresAt: jwt.NewNumericDate(
				now.Add(s.ttl),
			),

			NotBefore: jwt.NewNumericDate(now),

			ID: mustTokenID(),
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
				return nil, jwt.ErrTokenSignatureInvalid
			}

			return s.secret, nil
		},
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

func mustTokenID() string {
	id, err := randomtoken.Generate(16)

	if err != nil {
		panic(err)
	}

	return id
}
