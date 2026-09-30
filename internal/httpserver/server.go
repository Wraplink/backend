package httpserver

import (
	"backend/internal/auth"
	"backend/internal/security"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(
	registerHandler *auth.RegisterHandler,
	registrationRateLimitMiddleware func(http.Handler) http.Handler,
	cors *security.CORS,
) http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Middleware)
	r.Use(security.SecurityHeaders)

	r.Get("/health/live", func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		w.Header().Set(
			"Content-Type",
			"application/json",
		)

		w.WriteHeader(http.StatusOK)

		_, _ = w.Write([]byte(
			`{"status":"ok"}`,
		))
	})

	r.Route("/api/v1/auth", func(r chi.Router) {
		r.With(
			registrationRateLimitMiddleware,
		).Post("/register", registerHandler.Register)
	})

	return r
}
