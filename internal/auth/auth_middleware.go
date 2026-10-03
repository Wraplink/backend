package auth

import (
	"backend/internal/requestcontext"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type AuthMiddleware struct {
	tokenService *TokenService
}

func NewAuthMiddleware(
	tokenService *TokenService,
) *AuthMiddleware {
	if tokenService == nil {
		panic("auth: nil token service")
	}

	return &AuthMiddleware{
		tokenService: tokenService,
	}
}

func (m *AuthMiddleware) Middleware(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		token, ok := bearerToken(
			r.Header.Get("Authorization"),
		)

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

		ctx := requestcontext.WithUserID(
			r.Context(),
			userID,
		)

		next.ServeHTTP(
			w,
			r.WithContext(ctx),
		)
	})
}

func bearerToken(
	authorization string,
) (string, bool) {
	parts := strings.Fields(
		strings.TrimSpace(authorization),
	)

	if len(parts) != 2 {
		return "", false
	}

	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}

	if parts[1] == "" {
		return "", false
	}

	return parts[1], true
}
