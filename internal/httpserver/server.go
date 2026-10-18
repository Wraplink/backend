package httpserver

import (
	"backend/internal/auth"
	"backend/internal/dnsregistry"
	"backend/internal/networkaccess"
	"backend/internal/networkentitlement"
	"backend/internal/payments"
	"backend/internal/plans"
	"backend/internal/security"
	"backend/internal/subscriptions"
	"backend/internal/user"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(
	plansHandler *plans.Handler,
	subscriptionsHandler *subscriptions.Handler,
	paymentsHandler *payments.Handler,
	depositHandler *payments.DepositHandler,
	networkHandler *networkaccess.Handler,
	entitlementHandler *networkentitlement.Handler,
	dnsRegistryHandler *dnsregistry.Handler,
	registerHandler *auth.RegisterHandler,
	loginHandler *auth.LoginHandler,
	refreshHandler *auth.RefreshHandler,
	logoutHandler *auth.LogoutHandler,
	emailVerificationHandler *auth.EmailVerificationHandler,
	resendVerificationHandler *auth.ResendVerificationHandler,
	passwordResetHandler *auth.PasswordResetHandler,
	meHandler *user.MeHandler,
	passwordChangeHandler *auth.PasswordChangeHandler,
	sessionManagementHandler *auth.SessionManagementHandler,
	revokeOthersHandler *auth.RevokeOthersHandler,
	registrationRateLimitMiddleware func(http.Handler) http.Handler,
	resendVerificationRateLimitMiddleware func(http.Handler) http.Handler,
	requireAuthMiddleware func(http.Handler) http.Handler,
	passwordResetRateLimitMiddleware func(http.Handler) http.Handler,
	cors *security.CORS,
	logger *slog.Logger,
) http.Handler {
	r := chi.NewRouter()

	r.Use(security.RequestID)
	r.Use(security.Recovery(logger))
	r.Use(security.RequestLogger(logger))
	r.Use(cors.Middleware)
	r.Use(security.SecurityHeaders)

	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/api/v1/plans", plansHandler.List)
	r.Get("/api/v1/plans/{code}", plansHandler.Get)

	if depositHandler.DevelopmentSettlementEnabled() {
		r.Post("/api/v1/dev/deposits/{depositID}/settle", depositHandler.SettleDevelopment)
	}

	if networkHandler.UsageIngestEnabled() {
		r.Post("/api/v1/internal/usage/reports", networkHandler.IngestUsage)
	}

	if entitlementHandler.SnapshotEnabled() {
		r.Get(
			"/api/v1/internal/network/entitlements",
			entitlementHandler.Snapshot,
		)
	}

	if dnsRegistryHandler.Enabled() {
		r.Post(
			"/api/v1/internal/network/dns-controls/register",
			dnsRegistryHandler.Register,
		)
		r.Get(
			"/api/v1/internal/network/dns-controls",
			dnsRegistryHandler.List,
		)
	}

	r.Route("/api/v1/auth", func(r chi.Router) {
		r.With(registrationRateLimitMiddleware).Post("/register", registerHandler.Register)
		r.Post("/login", loginHandler.Login)
		r.Post("/verify-email", emailVerificationHandler.Verify)
		r.With(resendVerificationRateLimitMiddleware).Post("/resend-verification", resendVerificationHandler.Resend)
		r.Post("/refresh", refreshHandler.Refresh)
		r.With(requireAuthMiddleware).Post("/logout", logoutHandler.Logout)
		r.With(passwordResetRateLimitMiddleware).Post("/password-reset/request", passwordResetHandler.Request)
		r.Post("/password-reset/confirm", passwordResetHandler.Confirm)
	})

	r.Route("/api/v1/me", func(r chi.Router) {
		r.Use(requireAuthMiddleware)

		r.Get("/", meHandler.Get)
		r.Patch("/", meHandler.Update)
		r.Post("/password", passwordChangeHandler.Change)

		r.Get("/sessions", sessionManagementHandler.List)
		r.Delete("/sessions/{sessionID}", sessionManagementHandler.Revoke)
		r.Post("/sessions/revoke-others", revokeOthersHandler.Revoke)

		r.Post("/subscriptions", subscriptionsHandler.CreatePending)
		r.Get("/subscriptions", subscriptionsHandler.List)
		r.Get("/subscriptions/current", subscriptionsHandler.Current)
		r.Delete("/subscriptions/{subscriptionID}", subscriptionsHandler.CancelPending)
		r.Post("/subscriptions/{subscriptionID}/pay-with-wallet", paymentsHandler.PaySubscriptionWithWallet)

		r.Get("/network/access", networkHandler.Access)
		r.Post("/network/diagnostics/{type}", networkHandler.Diagnostic)
		r.Get("/network/events", entitlementHandler.Events)
		r.Get("/network/mode", entitlementHandler.GetMode)
		r.Put("/network/mode", entitlementHandler.SetMode)
		r.Post("/network/entitlement", entitlementHandler.Activate)
		r.Get("/network/entitlement", entitlementHandler.Current)
		r.Delete("/network/entitlement", entitlementHandler.Deactivate)
		r.Get("/usage", networkHandler.Usage)

		r.Get("/wallet", paymentsHandler.Wallet)
		r.Get("/wallet/transactions", paymentsHandler.Transactions)

		r.Post("/deposits", depositHandler.Create)
		r.Get("/deposits", depositHandler.List)
		r.Get("/deposits/{depositID}", depositHandler.Get)

		r.Get("/payments", paymentsHandler.Payments)
		r.Get("/payments/{paymentID}", paymentsHandler.Payment)
	})

	return r
}
