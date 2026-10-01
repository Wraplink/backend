package main

import (
	"backend/internal/auth"
	"backend/internal/security"
	"backend/internal/user"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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

	registrationRateLimiter := security.NewRateLimiter(
		5,
		time.Minute,
	)

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

	userRepository := user.NewRepository(db)

	tokenService := auth.NewTokenService(cfg)
	sessionService := auth.NewSessionService(db)
	loginService := auth.NewLoginService(
		userRepository,
		tokenService,
		sessionService,
		cfg.Security,
	)
	loginHandler := auth.NewLoginHandler(
		loginService,
	)

	logoutHandler := auth.NewLogoutHandler(
		sessionService,
	)

	emailVerificationService := auth.NewEmailVerificationService(
		db,
	)
	emailVerificationHandler := auth.NewEmailVerificationHandler(
		emailVerificationService,
	)
	refreshService := auth.NewRefreshService(
		sessionService,
		tokenService,
		cfg.Security,
	)

	resendVerificationEmailLimiter := security.NewRateLimiter(
		3,
		time.Hour,
	)

	resendVerificationRateLimiter := security.NewRateLimiter(
		3,
		15*time.Minute,
	)

	resendVerificationService := auth.NewResendVerificationService(
		db,
	)

	resendVerificationHandler := auth.NewResendVerificationHandler(
		resendVerificationService,
		resendVerificationEmailLimiter,
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
	refreshHandler := auth.NewRefreshHandler(
		refreshService,
	)

	registrationService := auth.NewRegistrationService(db)

	registerHandler := auth.NewRegisterHandler(
		registrationService,
	)

	router := httpserver.NewRouter(
		registerHandler,
		loginHandler,
		refreshHandler,
		logoutHandler,
		emailVerificationHandler,
		resendVerificationHandler,
		registrationRateLimitMiddleware,
		resendVerificationRateLimitMiddleware,
		cors,
		logger)

	server := &http.Server{
		Addr: cfg.HTTP.Host + ":" + cfg.HTTP.Port,

		Handler: router,

		ReadTimeout: cfg.HTTP.ReadTimeout,

		WriteTimeout: cfg.HTTP.WriteTimeout,

		IdleTimeout: cfg.HTTP.IdleTimeout,
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
