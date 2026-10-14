package main

import (
	"backend/internal/auth"
	"backend/internal/email"
	"backend/internal/networkaccess"
	"backend/internal/payments"
	"backend/internal/plans"
	"backend/internal/security"
	"backend/internal/subscriptions"
	"backend/internal/user"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/httpserver"
)

func main() {
	logger := slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			nil,
		),
	)

	slog.SetDefault(logger)

	cfg, err := config.Load(
		"config.yml",
	)
	if err != nil {
		logger.Error(
			"failed to load configuration",
			"error", err,
		)

		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	db, err := database.NewPostgres(
		ctx,
		cfg,
	)

	if err != nil {
		logger.Error(
			"failed to connect to database",
			"error", err,
		)

		os.Exit(1)
	}
	defer db.Close()

	cors := security.NewCORS(
		cfg.CORS.AllowedOrigins,
	)

	trustedProxy, err := security.NewTrustedProxy(
		cfg.Server.TrustedProxies,
	)
	if err != nil {
		logger.Error(
			"invalid trusted proxy configuration",
			"error", err,
		)
		return
	}

	emailSender := email.NewLogSender(logger, cfg.App.FrontendURL)

	// Repository
	plansRepository := plans.NewRepository(db)
	subscriptionsRepository := subscriptions.NewRepository(db)
	paymentsRepository := payments.NewRepository(db)
	networkRepository := networkaccess.NewRepository(db)
	userRepository := user.NewRepository(db)
	loginRepository := auth.NewLoginRepository(db)
	sessionRepository := auth.NewSessionRepository(db)
	passwordResetRepository := auth.NewPasswordResetRepository(db)
	passwordChangeRepository := auth.NewPasswordChangeRepository(db)

	// Service
	tokenService := auth.NewTokenService(cfg)
	sessionService := auth.NewSessionService(db)
	loginService := auth.NewLoginService(loginRepository, tokenService, sessionService, cfg.Security)
	logoutService := auth.NewLogoutService(sessionRepository)
	resendVerificationService := auth.NewResendVerificationService(db)
	emailVerificationService := auth.NewEmailVerificationService(db)
	refreshService := auth.NewRefreshService(sessionService, tokenService, cfg.Security)
	registrationService := auth.NewRegistrationService(db, emailSender)
	meService := user.NewMeService(userRepository)
	passwordResetService := auth.NewPasswordResetService(userRepository, passwordResetRepository, emailSender)
	passwordChangeService := auth.NewPasswordChangeService(passwordChangeRepository)
	sessionManagementService := auth.NewSessionManagementService(sessionRepository)
	revokeOthersService := auth.NewRevokeOthersService(sessionRepository)
	subscriptionsService := subscriptions.NewService(subscriptionsRepository, plansRepository)
	paymentsService := payments.NewService(paymentsRepository)

	connection := networkaccess.ConnectionDetails{
		DNSServers: append([]string(nil), cfg.Network.DNSServers...),
	}
	if strings.TrimSpace(cfg.Network.Proxy.Host) != "" {
		connection.Proxy = &networkaccess.ProxyConnection{
			Host:     cfg.Network.Proxy.Host,
			Port:     cfg.Network.Proxy.Port,
			Protocol: cfg.Network.Proxy.Protocol,
		}
	}
	networkService := networkaccess.NewService(networkRepository, connection)

	// Rate Limiter
	registrationRateLimiter := security.NewRateLimiter(5, time.Minute)
	resendVerificationEmailLimiter := security.NewRateLimiter(3, time.Hour)
	resendVerificationRateLimiter := security.NewRateLimiter(3, 15*time.Minute)
	passwordResetRateLimiter := security.NewRateLimiter(5, time.Minute)

	// Handler
	plansHandler := plans.NewHandler(plansRepository, logger)
	subscriptionsHandler := subscriptions.NewHandler(subscriptionsService)
	paymentsHandler := payments.NewHandler(paymentsService)
	depositHandler := payments.NewDepositHandler(
		paymentsService,
		cfg.Payments.DevSettlementKey,
		cfg.App.Environment != "production",
	)
	networkHandler := networkaccess.NewHandler(networkService, cfg.Network.UsageIngestKey)
	loginHandler := auth.NewLoginHandler(loginService, cfg.App.Environment == "production")
	logoutHandler := auth.NewLogoutHandler(logoutService, cfg.App.Environment == "production")
	resendVerificationHandler := auth.NewResendVerificationHandler(resendVerificationService, resendVerificationEmailLimiter)
	emailVerificationHandler := auth.NewEmailVerificationHandler(emailVerificationService)
	registerHandler := auth.NewRegisterHandler(registrationService)
	refreshHandler := auth.NewRefreshHandler(refreshService, cfg.App.Environment == "production")
	passwordResetHandler := auth.NewPasswordResetHandler(passwordResetService)
	meHandler := user.NewMeHandler(meService)
	passwordChangeHandler := auth.NewPasswordChangeHandler(passwordChangeService, trustedProxy)
	sessionManagementHandler := auth.NewSessionManagementHandler(sessionManagementService)
	revokeOthersHandler := auth.NewRevokeOthersHandler(revokeOthersService)

	// Middleware
	registrationRateLimitMiddleware :=
		registrationRateLimiter.Middleware(
			func(r *http.Request) string {
				ip := trustedProxy.ClientIP(r)

				if ip == nil {
					return "unknown"
				}

				return ip.String()
			},
		)

	resendVerificationRateLimitMiddleware :=
		resendVerificationRateLimiter.Middleware(
			func(r *http.Request) string {
				ip := trustedProxy.ClientIP(r)

				if ip == nil {
					return "unknown"
				}

				return ip.String()
			},
		)

	passwordResetRateLimitMiddleware :=
		passwordResetRateLimiter.Middleware(
			func(r *http.Request) string {
				ip := trustedProxy.ClientIP(r)

				if ip == nil {
					return "unknown"
				}

				return ip.String()
			},
		)

	// Authentication
	authMiddleware := auth.NewAuthMiddleware(tokenService, sessionService)
	requireAuthMiddleware := authMiddleware.Middleware

	router := httpserver.NewRouter(
		plansHandler,
		subscriptionsHandler,
		paymentsHandler,
		depositHandler,
		networkHandler,
		registerHandler,
		loginHandler,
		refreshHandler,
		logoutHandler,
		emailVerificationHandler,
		resendVerificationHandler,
		passwordResetHandler,
		meHandler,
		passwordChangeHandler,
		sessionManagementHandler,
		revokeOthersHandler,
		registrationRateLimitMiddleware,
		resendVerificationRateLimitMiddleware,
		requireAuthMiddleware,
		passwordResetRateLimitMiddleware,
		cors,
		logger)

	server := &http.Server{
		Addr:         cfg.HTTP.Host + ":" + cfg.HTTP.Port,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	serverErr := make(chan error, 1)

	go func() {
		logger.Info(
			"HTTP server started",
			"address", server.Addr,
		)

		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(
			err,
			http.ErrServerClosed,
		) {
			logger.Error(
				"HTTP server failed",
				"error", err,
			)

			os.Exit(1)
		}

	case <-ctx.Done():
		logger.Info(
			"shutdown signal received",
		)
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.HTTP.ShutdownTimeout,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error(
			"HTTP server shutdown failed",
			"error", err,
		)

		os.Exit(1)
	}

	logger.Info("HTTP server stopped")
}
