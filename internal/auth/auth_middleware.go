package auth

import (
	"backend/internal/requestcontext"
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type ActiveSessionChecker interface {
	IsActive(
		ctx context.Context,
		userID uuid.UUID,
		sessionID uuid.UUID,
	) (bool, error)
}

type AuthMiddleware struct {
	tokenService   *TokenService
	sessionChecker ActiveSessionChecker
}

func NewAuthMiddleware(
	tokenService *TokenService,
	sessionChecker ActiveSessionChecker,
) *AuthMiddleware {
	if tokenService == nil {
		panic("auth: nil token service")
	}
	if sessionChecker == nil {
		panic("auth: nil session checker")
	}

	return &AuthMiddleware{
		tokenService:   tokenService,
		sessionChecker: sessionChecker,
	}
}

func (m *AuthMiddleware) Middleware(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeJSONError(
				w,
				http.StatusUnauthorized,
				"authentication required",
			)
			return
		}

		claims, err := m.tokenService.ParseAccessToken(token)
		if err != nil {
			writeJSONError(
				w,
				http.StatusUnauthorized,
				"invalid or expired access token",
			)
			return
		}

		userID, err := uuid.Parse(claims.UserID)
		if err != nil || userID == uuid.Nil {
			writeJSONError(
				w,
				http.StatusUnauthorized,
				"invalid access token",
			)
			return
		}

		sessionID, err := uuid.Parse(claims.SessionID)
		if err != nil || sessionID == uuid.Nil {
			writeJSONError(
				w,
				http.StatusUnauthorized,
				"invalid access token",
			)
			return
		}

		active, err := m.sessionChecker.IsActive(
			r.Context(),
			userID,
			sessionID,
		)
		if err != nil {
			writeJSONError(
				w,
				http.StatusInternalServerError,
				"internal server error",
			)
			return
		}

		if !active {
			writeJSONError(
				w,
				http.StatusUnauthorized,
				"session expired or revoked",
			)
			return
		}

		ctx := requestcontext.WithUserID(
			r.Context(),
			userID,
		)
		ctx = requestcontext.WithSessionID(
			ctx,
			sessionID,
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(authorization string) (string, bool) {
	parts := strings.Fields(strings.TrimSpace(authorization))

	if len(parts) != 2 ||
		!strings.EqualFold(parts[0], "Bearer") ||
		parts[1] == "" {
		return "", false
	}

	return parts[1], true
}
