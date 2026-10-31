package main

import (
	"backend/internal/auth"
	"backend/internal/bootstrap"
	"backend/internal/config"
	"backend/internal/dashboard"
	"backend/internal/database"
	"backend/internal/dnsregistry"
 "backend/internal/dnsdomains"
	"backend/internal/email"
	"backend/internal/httpserver"
	"backend/internal/networkaccess"
 "backend/internal/routeinventory"
	"backend/internal/networkentitlement"
	"backend/internal/payments"
	"backend/internal/plans"
	"backend/internal/security"
	"backend/internal/subscriptions"
 "backend/internal/support"
 "backend/internal/notifications"
	"backend/internal/user"
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
 "strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	configFile := flag.String("config", "config.yml", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.NewPostgres(ctx, cfg)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := bootstrap.EnsureDefaultAdmin(ctx, db, logger); err != nil {
		logger.Error("failed to bootstrap administrator", "error", err)
		os.Exit(1)
	}

	if err := bootstrap.EnsureRequiredData(ctx, db, logger); err != nil {
		logger.Error("failed to bootstrap required data", "error", err)
		os.Exit(1)
	}

	// Retry route allocation independently of agent heartbeats. A transient
	// upstream DNS failure must not leave an otherwise healthy route waiting
	// indefinitely until another inventory registration occurs.
	go func() {
		reconcile := func() {
			runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			if err := routeinventory.Reconcile(runCtx, db, cfg.Network.ControlNodeOfflineAfter, cfg.Network.DNSControlURL, cfg.Network.DNSControlToken); err != nil {
				logger.Warn("route reconciliation failed", "error", err)
			}
		}
		reconcile() // recover existing unassigned policies after backend restart
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reconcile()
			}
		}
	}()

	cors := security.NewCORS(cfg.CORS.AllowedOrigins)
	trustedProxy, err := security.NewTrustedProxy(cfg.Server.TrustedProxies)
	if err != nil {
		logger.Error("invalid trusted proxy configuration", "error", err)
		os.Exit(1)
	}

	var emailSender email.Sender
 if strings.TrimSpace(os.Getenv("SMTP_HOST")) != "" {
  port := 587
  if raw:=os.Getenv("SMTP_PORT");raw!="" {
   parsed,err:=strconv.Atoi(raw)
   if err!=nil {logger.Error("invalid SMTP_PORT");os.Exit(1)}
   port=parsed
  }
  sender,err:=email.NewSMTPSender(email.SMTPConfig{
   Host:os.Getenv("SMTP_HOST"),Port:port,
   Username:os.Getenv("SMTP_USERNAME"),Password:os.Getenv("SMTP_PASSWORD"),
   From:os.Getenv("SMTP_FROM"),FrontendURL:cfg.App.FrontendURL,
  })
  if err!=nil {logger.Error("invalid SMTP configuration","error",err);os.Exit(1)}
  emailSender=sender
 } else if strings.EqualFold(cfg.App.Environment,"production") {
  logger.Error("SMTP_HOST must be configured in production; refusing to log email verification tokens")
  os.Exit(1)
 } else {
  emailSender=email.NewLogSender(logger,cfg.App.FrontendURL)
 }

	// Repositories.
	plansRepository := plans.NewRepository(db)
	subscriptionsRepository := subscriptions.NewRepository(db)
	paymentsRepository := payments.NewRepository(db)
	networkRepository := networkaccess.NewRepository(db)
	entitlementRepository := networkentitlement.NewRepository(db)
	userRepository := user.NewRepository(db)
	loginRepository := auth.NewLoginRepository(db)
	sessionRepository := auth.NewSessionRepository(db)
	passwordResetRepository := auth.NewPasswordResetRepository(db)
	passwordChangeRepository := auth.NewPasswordChangeRepository(db)

	// Runtime control-plane registry. dns-control nodes refresh this registry
	// periodically, so backend restarts recover without database state.
	dnsControlStore := dnsregistry.NewStore()

	// Services.
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
	entitlementService := networkentitlement.NewService(entitlementRepository)

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
	networkService := networkaccess.NewService(
		networkRepository,
		connection,
		dnsControlStore,
		cfg.Network.ControlNodeOfflineAfter,
	)

	// Rate limiters.
	registrationRateLimiter := security.NewRateLimiter(5, time.Minute)
	resendVerificationEmailLimiter := security.NewRateLimiter(3, time.Hour)
	resendVerificationRateLimiter := security.NewRateLimiter(3, 15*time.Minute)
	passwordResetRateLimiter := security.NewRateLimiter(5, time.Minute)

	// Handlers.
	plansHandler := plans.NewHandler(plansRepository, logger)
	subscriptionsHandler := subscriptions.NewHandler(subscriptionsService)
 supportHandler := support.NewHandler(db)
 notificationsHandler := notifications.NewHandler(db)
	paymentsHandler := payments.NewHandler(paymentsService)
	depositHandler := payments.NewDepositHandler(
		paymentsService,
		cfg.Payments.DevSettlementKey,
		cfg.App.Environment != "production",
	)
	networkHandler := networkaccess.NewHandler(networkService, cfg.Network.UsageIngestKey)
	dashboardHandler := dashboard.NewHandler(paymentsService, networkService, entitlementService)
	entitlementHandler := networkentitlement.NewHandler(
		entitlementService,
		trustedProxy,
		cfg.Network.ControlSyncKey,
	)
	dnsRegistryHandler := dnsregistry.NewHandler(
		dnsControlStore,
		cfg.Network.ControlSyncKey,
		cfg.Network.ControlNodeOfflineAfter,
        db,
	)
	dnsRegistryHandler.SetRouteDNSControl(cfg.Network.DNSControlURL, cfg.Network.DNSControlToken)
	routeInventoryHandler := routeinventory.Handler{DB:db,ControlKey:cfg.Network.ControlSyncKey,OfflineAfter:cfg.Network.ControlNodeOfflineAfter,DNSControlURL:cfg.Network.DNSControlURL,DNSControlToken:cfg.Network.DNSControlToken}
	dnsDomainsHandler := &dnsdomains.Handler{Store:dnsdomains.Store{DB:db},ControlKey:cfg.Network.ControlSyncKey}
 loginHandler := auth.NewLoginHandler(loginService, cfg.App.Environment == "production")
	logoutHandler := auth.NewLogoutHandler(logoutService, cfg.App.Environment == "production")
	resendVerificationHandler := auth.NewResendVerificationHandler(resendVerificationService, resendVerificationEmailLimiter, emailSender)
	emailVerificationHandler := auth.NewEmailVerificationHandler(emailVerificationService)
	registerHandler := auth.NewRegisterHandler(registrationService)
	refreshHandler := auth.NewRefreshHandler(refreshService, cfg.App.Environment == "production")
	passwordResetHandler := auth.NewPasswordResetHandler(passwordResetService)
	meHandler := user.NewMeHandler(meService)
	passwordChangeHandler := auth.NewPasswordChangeHandler(passwordChangeService, trustedProxy)
	sessionManagementHandler := auth.NewSessionManagementHandler(sessionManagementService)
	revokeOthersHandler := auth.NewRevokeOthersHandler(revokeOthersService)

	registrationRateLimitMiddleware := registrationRateLimiter.Middleware(func(r *http.Request) string {
		ip := trustedProxy.ClientIP(r)
		if ip == nil {
			return "unknown"
		}
		return ip.String()
	})
	resendVerificationRateLimitMiddleware := resendVerificationRateLimiter.Middleware(func(r *http.Request) string {
		ip := trustedProxy.ClientIP(r)
		if ip == nil {
			return "unknown"
		}
		return ip.String()
	})
	passwordResetRateLimitMiddleware := passwordResetRateLimiter.Middleware(func(r *http.Request) string {
		ip := trustedProxy.ClientIP(r)
		if ip == nil {
			return "unknown"
		}
		return ip.String()
	})

	authMiddleware := auth.NewAuthMiddleware(tokenService, sessionService)
	requireAuthMiddleware := authMiddleware.Middleware

	router := httpserver.NewRouter(
		plansHandler,
        supportHandler,
        notificationsHandler,
		subscriptionsHandler,
		paymentsHandler,
		depositHandler,
		networkHandler,
		dashboardHandler,
		entitlementHandler,
		dnsRegistryHandler,
        routeInventoryHandler,
        dnsDomainsHandler,
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
		logger,
	)

	server := &http.Server{
		Addr:         cfg.HTTP.Host + ":" + cfg.HTTP.Port,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server started", "address", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("HTTP server stopped")
}
