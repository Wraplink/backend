package auth

import (
	"backend/internal/legal"
	"backend/internal/security"
	"backend/internal/token"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

const googleCallbackPath = "/api/v1/auth/google/callback"

type GoogleAuthHandler struct {
	db           *pgxpool.Pool
	login        *LoginService
	logger       *slog.Logger
	client       *http.Client
	frontendURL  string
	clientID     string
	clientSecret string
	redirectURI  string
	secure       bool
}

func NewGoogleAuthHandler(db *pgxpool.Pool, login *LoginService, logger *slog.Logger, frontendURL, clientID, clientSecret, redirectURI string, secure bool) *GoogleAuthHandler {
	return &GoogleAuthHandler{
		db: db, login: login, logger: logger,
		frontendURL: strings.TrimRight(frontendURL, "/"),
		clientID:    strings.TrimSpace(clientID), clientSecret: strings.TrimSpace(clientSecret),
		redirectURI: strings.TrimSpace(redirectURI), secure: secure,
		client: &http.Client{
			Timeout:       5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (h *GoogleAuthHandler) configured() bool {
	callback, err := url.Parse(h.redirectURI)
	return h.clientID != "" && h.clientSecret != "" && err == nil &&
		callback.Hostname() != "" && callback.Path == googleCallbackPath &&
		callback.User == nil && callback.RawQuery == "" && callback.Fragment == "" &&
		(callback.Scheme == "https" || (!h.secure && callback.Scheme == "http"))
}

func (h *GoogleAuthHandler) enabled(ctx context.Context) (bool, error) {
	var enabled bool
	err := h.db.QueryRow(ctx, "SELECT enabled FROM google_auth_settings WHERE singleton").Scan(&enabled)
	return enabled, err
}

func (h *GoogleAuthHandler) Availability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	enabled, err := h.enabled(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "Google sign-in unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled && h.configured()})
}

func (h *GoogleAuthHandler) AdminSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	enabled, err := h.enabled(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "Google settings unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled, "configured": h.configured()})
}

func (h *GoogleAuthHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeGoogleRequest(w, r, &input) || input.Enabled == nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if *input.Enabled && !h.configured() {
		writeJSONError(w, http.StatusConflict, "Google OAuth credentials are missing or invalid")
		return
	}
	if _, err := h.db.Exec(r.Context(), "UPDATE google_auth_settings SET enabled = $1 WHERE singleton", *input.Enabled); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "Google settings unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": *input.Enabled, "configured": h.configured()})
}

type googleStartRequest struct {
	Locale           string `json:"locale"`
	ReturnURL        string `json:"returnUrl"`
	RememberDevice   bool   `json:"rememberDevice"`
	TermsVersion     string `json:"termsVersion"`
	PrivacyVersion   string `json:"privacyVersion"`
	MarketingConsent bool   `json:"marketingConsent"`
}

type googleFlow struct {
	googleStartRequest
	Register  bool
	Verifier  string
	ExpiresAt time.Time
}

func (h *GoogleAuthHandler) StartLogin(w http.ResponseWriter, r *http.Request) {
	h.start(w, r, false)
}

func (h *GoogleAuthHandler) StartRegistration(w http.ResponseWriter, r *http.Request) {
	h.start(w, r, true)
}

func (h *GoogleAuthHandler) start(w http.ResponseWriter, r *http.Request, register bool) {
	w.Header().Set("Cache-Control", "no-store")
	frontend, err := url.Parse(h.frontendURL)
	if err != nil || r.Header.Get("Origin") != frontend.Scheme+"://"+frontend.Host {
		writeJSONError(w, http.StatusForbidden, "invalid origin")
		return
	}
	enabled, err := h.enabled(r.Context())
	if err != nil || !enabled || !h.configured() {
		writeJSONError(w, http.StatusServiceUnavailable, "Google sign-in unavailable")
		return
	}

	var input googleStartRequest
	if !decodeGoogleRequest(w, r, &input) || validateLocale(input.Locale) != nil ||
		(register && (input.TermsVersion != legal.CurrentTermsVersion || input.PrivacyVersion != legal.CurrentPrivacyVersion)) {
		writeJSONError(w, http.StatusBadRequest, "invalid request")
		return
	}
	input.ReturnURL = safeGoogleReturnURL(input.ReturnURL)
	if register {
		input.RememberDevice = false
	} else {
		input.TermsVersion = ""
		input.PrivacyVersion = ""
		input.MarketingConsent = false
	}
	verifier, err := token.Generate(32)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "unable to start Google sign-in")
		return
	}
	flow := googleFlow{googleStartRequest: input, Register: register, Verifier: verifier, ExpiresAt: time.Now().Add(10 * time.Minute)}
	payload, err := json.Marshal(flow)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "unable to start Google sign-in")
		return
	}
	cookie := base64.RawURLEncoding.EncodeToString(payload)
	if len(cookie) > 3800 {
		writeJSONError(w, http.StatusBadRequest, "invalid return URL")
		return
	}
	// State binds this authorization request to the browser's host-only flow
	// cookie. The cookie also protects the verifier used in the token exchange.
	h.setFlowCookie(w, cookie, 600)
	query := url.Values{
		"client_id": {h.clientID}, "redirect_uri": {h.redirectURI}, "response_type": {"code"},
		"scope": {"openid email profile"}, "state": {token.Hash(cookie)},
		"code_challenge": {googleCodeChallenge(verifier)}, "code_challenge_method": {"S256"},
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorizationUrl": "https://accounts.google.com/o/oauth2/v2/auth?" + query.Encode()})
}

func (h *GoogleAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")

	query := r.URL.Query()
	flow, ok := h.readFlow(r, query.Get("state"))
	if !ok {
		h.redirect(w, r, googleFlow{}, googleFailed)
		return
	}
	h.setFlowCookie(w, "", -1)
	enabled, err := h.enabled(r.Context())
	code := query.Get("code")
	if err != nil || !enabled || !h.configured() || code == "" || query.Get("error") != "" {
		h.redirect(w, r, flow, googleFailed)
		return
	}
	profile, err := h.fetchGoogleProfile(r.Context(), code, flow.Verifier)
	if err != nil {
		h.logger.Warn("Google authorization could not be verified")
		h.redirect(w, r, flow, googleFailed)
		return
	}

	enabled, err = h.enabled(r.Context())
	if err != nil || !enabled {
		h.redirect(w, r, flow, googleFailed)
		return
	}
	req := LoginRequest{IPAddress: parseIP(security.RemoteIP(r)), UserAgent: r.UserAgent()}
	account, err := h.account(r.Context(), profile, flow, req)
	if errors.Is(err, errGoogleRegistrationRequired) {
		h.redirect(w, r, flow, googleNeedsRegistration)
		return
	}
	if err != nil {
		if !errors.Is(err, ErrInvalidCredentials) && !errors.Is(err, ErrInvalidRegistration) && !isUniqueViolation(err) {
			h.logger.Error("Google account lookup or creation failed")
		}
		h.redirect(w, r, flow, googleFailed)
		return
	}
	result, err := h.login.createLoginSession(r.Context(), account, req)
	if err != nil {
		h.logger.Error("Google login session creation failed")
		h.redirect(w, r, flow, googleFailed)
		return
	}
	setRefreshCookieWithPersistence(w, result.RefreshToken, result.RefreshUntil, h.secure, flow.RememberDevice)
	setRememberDeviceCookie(w, flow.RememberDevice, result.RefreshUntil, h.secure)
	h.redirect(w, r, flow, googleSignedIn)
}

func (h *GoogleAuthHandler) flowCookieName() string {
	if h.secure {
		return "__Host-wraplink_google_flow"
	}
	return "wraplink_google_flow"
}

func (h *GoogleAuthHandler) setFlowCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: h.flowCookieName(), Value: value, MaxAge: maxAge, Path: "/",
		HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode,
	})
}

func (h *GoogleAuthHandler) readFlow(r *http.Request, state string) (googleFlow, bool) {
	var flow googleFlow
	cookie, err := r.Cookie(h.flowCookieName())
	if err != nil || subtle.ConstantTimeCompare([]byte(token.Hash(cookie.Value)), []byte(state)) != 1 {
		return flow, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || json.Unmarshal(payload, &flow) != nil || flow.Verifier == "" || !time.Now().Before(flow.ExpiresAt) {
		return googleFlow{}, false
	}
	return flow, true
}

type googleRedirectOutcome int

const (
	googleSignedIn googleRedirectOutcome = iota
	googleFailed
	googleNeedsRegistration
)

func (h *GoogleAuthHandler) redirect(w http.ResponseWriter, r *http.Request, flow googleFlow, outcome googleRedirectOutcome) {
	path := "/auth/login"
	if outcome == googleNeedsRegistration || (outcome == googleFailed && flow.Register) {
		path = "/auth/register"
	}
	if flow.Locale == "en" {
		path = "/en" + path
	}
	query := url.Values{}
	if destination := safeGoogleReturnURL(flow.ReturnURL); destination != "" {
		query.Set("returnUrl", destination)
	}
	switch outcome {
	case googleFailed:
		query.Set("googleError", "1")
	case googleNeedsRegistration:
		query.Set("googleSignup", "1")
	}
	target := h.frontendURL + path
	if len(query) != 0 {
		target += "?" + query.Encode()
	}
	if outcome == googleNeedsRegistration {
		target += "#agreements"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func safeGoogleReturnURL(value string) string {
	if !strings.HasPrefix(value, "/") || len(value) > 1000 || strings.ContainsFunc(value, unicode.IsControl) {
		return ""
	}
	target, err := url.Parse(value)
	if err != nil || target.Host != "" || strings.HasPrefix(value, "//") ||
		strings.HasPrefix(target.Path, "//") || strings.Contains(target.Path, "\\") ||
		strings.ContainsFunc(target.Path, unicode.IsControl) {
		return ""
	}
	return value
}

func googleCodeChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func decodeGoogleRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && errors.Is(decoder.Decode(new(any)), io.EOF)
}
