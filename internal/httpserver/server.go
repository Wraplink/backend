package httpserver

import (
	"backend/internal/auth"
	"backend/internal/security"
	"backend/internal/user"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(
	registerHandler *auth.RegisterHandler,
	loginHandler *auth.LoginHandler,
	refreshHandler *auth.RefreshHandler,
	logoutHandler *auth.LogoutHandler,
	emailVerificationHandler *auth.EmailVerificationHandler,
	resendVerificationHandler *auth.ResendVerificationHandler,
	meHandler *user.MeHandler,
	registrationRateLimitMiddleware func(http.Handler) http.Handler,
	resendVerificationRateLimitMiddleware func(http.Handler) http.Handler,
	requireAuthMiddleware func(http.Handler) http.Handler,
	cors *security.CORS,
	logger *slog.Logger,
) http.Handler {
	r := chi.NewRouter()

	r.Use(security.RequestID)
	r.Use(security.Recovery(logger))
	r.Use(security.RequestLogger(logger))
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
		r.With(registrationRateLimitMiddleware).Post("/register", registerHandler.Register)
		r.Post("/login", loginHandler.Login)
		r.Post("/verify-email", emailVerificationHandler.Verify)
		r.With(resendVerificationRateLimitMiddleware).Post("/resend-verification", resendVerificationHandler.Resend)

		r.Post("/refresh", refreshHandler.Refresh)

		r.With(requireAuthMiddleware).Post("/logout", logoutHandler.Logout)
	})

	r.Route("/api/v1/me", func(r chi.Router) {
		r.Use(requireAuthMiddleware)

		r.Get("/", meHandler.Get)
		r.Patch("/", meHandler.Update)
	})

	return r
}
