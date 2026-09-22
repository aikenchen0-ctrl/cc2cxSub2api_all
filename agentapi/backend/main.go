package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxJSONBody = 2 << 20
const maxModelBody = 32 << 20

type Server struct {
	cfg    Config
	store  *Store
	main   *MainClient
	webDir string

	// Balance-delta settlement is serialized per main user. Without this lock,
	// two concurrent requests could both observe the same before/after balance.
	settleMu     sync.Mutex
	settleByUser map[string]*sync.Mutex
}

type apiResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Reason    string `json:"reason,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Data      any    `json:"data,omitempty"`
}

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}
	store, err := OpenStore(cfg.DatabasePath, cfg.SessionSecret)
	if err != nil {
		slog.Error("open store failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if err := store.UpsertAgent(cfg); err != nil {
		slog.Error("initialize agent failed", "error", err)
		os.Exit(1)
	}

	server := &Server{
		cfg: cfg, store: store, main: NewMainClient(cfg), webDir: cfg.WebDir,
		settleByUser: make(map[string]*sync.Mutex),
	}
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       2 * time.Minute,
	}
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	go server.runSettlementReconciler(shutdownCtx)
	go func() {
		<-shutdownCtx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()
	slog.Info("agentapi started", "addr", cfg.Addr, "agent_id", cfg.AgentID, "domain", cfg.AgentDomain)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("agentapi stopped", "error", err)
		os.Exit(1)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := requestID(r)
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && !s.allowedHost(r.Host) {
		s.writeError(w, http.StatusMisdirectedRequest, requestID, "HOST_NOT_ALLOWED", "request host is not configured for this AgentAPI instance")
		return
	}
	s.setCORS(w, r)
	if r.Method == http.MethodOptions {
		if strings.TrimSpace(r.Header.Get("Origin")) != "" && !sameOriginOrMachine(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CORS_ORIGIN_REJECTED", "origin is not allowed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch {
	case r.URL.Path == "/healthz":
		s.writeData(w, http.StatusOK, requestID, map[string]any{"status": "ok", "agent_id": s.cfg.AgentID})
	case r.URL.Path == "/readyz":
		s.handleReady(w, r, requestID)
	case r.URL.Path == "/api/auth/sso/callback" || r.URL.Path == "/api/v1/auth/sso/callback":
		s.handleSSOCallback(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/auth/"):
		s.handleAuth(w, r, requestID)
	case r.URL.Path == "/api/v1/settings/public":
		s.handlePublicSettings(w, r, requestID)
	case r.URL.Path == "/api/v1/payments/webhook":
		// Payment providers call this endpoint without an AgentAPI cookie. Keep
		// it before the generic /api/v1 branch and authenticate it with the
		// provider-neutral HMAC contract in handlePaymentWebhook.
		s.handlePaymentWebhook(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/context":
		s.handleAgentContext(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/wallet":
		s.handleAgentWallet(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/users":
		s.handleAgentUsers(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/usage":
		s.handleAgentUsage(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/recharge/orders":
		s.handleAgentRecharge(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/agent/admin/"):
		s.handleAgentAdmin(w, r, requestID)
	case r.URL.Path == "/api/v1/api-keys" || strings.HasPrefix(r.URL.Path, "/api/v1/api-keys/"):
		s.handleAPIKeys(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/"):
		s.handleMainAPIProxy(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/v1/"):
		s.handleModelRelay(w, r, requestID)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		s.serveWeb(w, r)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
	}
}

func (s *Server) allowedHost(rawHost string) bool {
	expected := strings.TrimSpace(s.cfg.AgentDomain)
	if expected == "" {
		return true
	}
	normalize := func(value string) string {
		value = strings.TrimSpace(strings.ToLower(value))
		if host, _, err := net.SplitHostPort(value); err == nil {
			value = host
		} else {
			value = strings.Trim(value, "[]")
		}
		return strings.TrimSuffix(value, ".")
	}
	return normalize(rawHost) == normalize(expected)
}

func (s *Server) runSettlementReconciler(ctx context.Context) {
	interval := s.cfg.SettlementReconcileInterval
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcilePendingInBackground(ctx)
		}
	}
}

func (s *Server) reconcilePendingInBackground(parent context.Context) {
	if strings.TrimSpace(s.cfg.AdminKey) == "" || strings.TrimSpace(s.cfg.OwnerMainUserID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(parent, s.cfg.MainRequestTimeout)
	defer cancel()
	items, err := s.reconcileSettlements(ctx, "")
	if err != nil {
		slog.Warn("background settlement reconciliation failed", "agent_id", s.cfg.AgentID, "error", err)
	} else if len(items) > 0 {
		slog.Info("background settlement reconciliation completed", "agent_id", s.cfg.AgentID, "count", len(items))
	}
	s.reconcilePaidRechargeOrders(ctx)
	s.reconcileStaleVideoTasks(ctx)
}

func (s *Server) reconcileStaleVideoTasks(ctx context.Context) {
	if s.cfg.VideoTaskReconcileAge <= 0 || strings.TrimSpace(s.cfg.OwnerMainUserID) == "" || strings.TrimSpace(s.cfg.AppCredential) == "" {
		return
	}
	tasks, err := s.store.PendingVideoTasks(s.cfg.AgentID, s.cfg.SettlementReconcileBatch)
	if err != nil {
		slog.Warn("video task reconciliation lookup failed", "agent_id", s.cfg.AgentID, "error", err)
		return
	}
	cutoff := time.Now().UTC().Add(-s.cfg.VideoTaskReconcileAge)
	for _, task := range tasks {
		createdAt, parseErr := time.Parse(time.RFC3339, task.CreatedAt)
		if parseErr != nil || createdAt.After(cutoff) {
			continue
		}
		ownerID := strings.TrimSpace(s.cfg.OwnerMainUserID)
		mu := s.userSettlementMutex(ownerID)
		mu.Lock()
		status, _, body, relayErr := s.main.RelayModelMethod(ctx, http.MethodGet, "/v1/videos/"+url.PathEscape(task.TaskID), nil, nil, ownerID, task.RequestID)
		shouldReconcile := false
		if relayErr == nil && status >= 200 && status < 300 {
			_, upstreamStatus := videoTaskIdentity(body)
			if upstreamStatus != "" {
				_ = s.store.UpdateVideoTaskStatus(s.cfg.AgentID, task.TaskID, upstreamStatus)
				if videoTaskFailed(upstreamStatus) {
					_ = s.store.FinalizeSettlement(s.cfg.AgentID, task.RequestID, task.RequestID, "released", 0, "video task expired without a charge: "+upstreamStatus)
				} else {
					shouldReconcile = true
				}
			}
		}
		mu.Unlock()
		if shouldReconcile {
			_, _ = s.reconcileSettlements(ctx, task.RequestID)
		}
		if relayErr != nil {
			slog.Warn("stale video task probe failed", "agent_id", s.cfg.AgentID, "task_id", task.TaskID, "error", relayErr)
		}
	}
}

func (s *Server) reconcilePaidRechargeOrders(ctx context.Context) {
	if !s.cfg.PaymentEnabled || strings.TrimSpace(s.cfg.OwnerMainUserID) == "" || strings.TrimSpace(s.cfg.AdminKey) == "" {
		return
	}
	orders, err := s.store.PaidPendingRechargeOrders(s.cfg.AgentID, s.cfg.SettlementReconcileBatch)
	if err != nil {
		slog.Warn("background recharge reconciliation lookup failed", "agent_id", s.cfg.AgentID, "error", err)
		return
	}
	for _, order := range orders {
		if _, err := s.syncAndAllocateRechargeOrder(ctx, "payment-reconcile:"+order.OrderNo, order.OrderNo); err != nil {
			if errors.Is(err, errInsufficientBalance) {
				// Keep the order pending until the owner tops up. Do not turn a
				// verified payment into a failure merely because credit is delayed.
				continue
			}
			slog.Warn("background recharge allocation failed", "agent_id", s.cfg.AgentID, "order_no", order.OrderNo, "error", err)
			continue
		}
		slog.Info("background recharge allocation completed", "agent_id", s.cfg.AgentID, "order_no", order.OrderNo)
	}
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	agent, err := s.store.Agent(s.cfg.AgentID)
	if err != nil || agent.Status != "active" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "AGENT_NOT_READY", "agent configuration is not ready")
		return
	}
	if strings.TrimSpace(s.cfg.AppCredential) == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "MAIN_APP_CREDENTIAL_MISSING", "model relay credential is not configured")
		return
	}
	if s.cfg.BillingMode != "" && s.cfg.BillingMode != "owner_upstream" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "BILLING_MODE_INVALID", "AgentAPI billing mode is invalid")
		return
	}
	if strings.TrimSpace(s.cfg.OwnerMainUserID) == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "MAIN_OWNER_MISSING", "billing owner is not configured")
		return
	}
	if strings.TrimSpace(s.cfg.AdminKey) == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "MAIN_ADMIN_KEY_MISSING", "main administrator credential is not configured")
		return
	}
	if s.cfg.SessionSecretWeak {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "SESSION_SECRET_WEAK", "session secret must contain at least 32 characters")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"status": "ready", "agent_id": s.cfg.AgentID, "domain": agent.Domain})
}

func (s *Server) setCORS(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return
	}
	// AgentAPI is normally same-origin. Reflect an origin only after checking it
	// against the request Host; reflecting arbitrary origins with cookies
	// would let an unrelated site read wallet and account data.
	if !sameOriginOrMachine(r) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, Idempotency-Key")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
	_ = r
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request, requestID string) {
	switch r.URL.Path {
	case "/api/v1/auth/register":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authRegister(w, r, requestID)
	case "/api/v1/auth/login":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authLogin(w, r, requestID)
	case "/api/v1/auth/login/2fa":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authLogin2FA(w, r, requestID)
	case "/api/v1/auth/me":
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authMe(w, r, requestID)
	case "/api/v1/auth/refresh":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authRefresh(w, r, requestID)
	case "/api/v1/auth/logout":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authLogout(w, r, requestID)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "authentication endpoint not found")
	}
}

// handleSSOCallback consumes the public Sub2API satellite ticket format. The
// ticket carries only the authenticated subject; it never carries a JWT,
// SuperKey, or an API key. AgentAPI creates its own three-day HttpOnly session
// and uses the subject as the server-side on-behalf-of identity for model
// requests.
func (s *Server) handleSSOCallback(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	secret := strings.TrimSpace(s.cfg.SSOSecret)
	if len(secret) < 32 {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "SSO_NOT_CONFIGURED", "SSO is not configured")
		return
	}
	ticket, err := verifySSOTicket(r.URL.Query().Get("ticket"), secret, s.cfg.SSOAudience, time.Now().UTC())
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, errIdempotencyConflict) {
			status = http.StatusConflict
		}
		s.writeError(w, status, requestID, "SSO_TICKET_INVALID", err.Error())
		return
	}
	if err := s.store.ConsumeSSOTicket(ticket.JTI, ticket.ExpiresAt); err != nil {
		status := http.StatusInternalServerError
		reason := "SSO_TICKET_CONSUME_FAILED"
		if errors.Is(err, errIdempotencyConflict) {
			status = http.StatusConflict
			reason = "SSO_TICKET_REPLAYED"
		}
		s.writeError(w, status, requestID, reason, "SSO ticket has already been used or cannot be consumed")
		return
	}
	userJSON, err := ticketUserJSON(ticket)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "SSO_USER_INVALID", err.Error())
		return
	}
	if existing, lookupErr := s.store.User(s.cfg.AgentID, ticket.Subject); lookupErr == nil {
		if existing.Status != "active" {
			s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_DISABLED", "agent user is disabled")
			return
		}
		if email, displayName, _ := userJSONFields(userJSON); email != "" || displayName != "" {
			_, _ = s.store.UpsertUser(s.cfg.AgentID, ticket.Subject, email, displayName)
		}
	} else if errors.Is(lookupErr, errNotFound) {
		if _, err := s.store.UpsertUser(s.cfg.AgentID, ticket.Subject, ticket.Email, ticket.DisplayName); err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to create SSO user mapping")
			return
		}
	} else {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load SSO user mapping")
		return
	}
	sessionID, err := s.store.CreateSession(ticket.Subject, userJSON, "", "", time.Now().UTC().Add(sessionTTL))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "SESSION_CREATE_FAILED", "failed to create session")
		return
	}
	expiresAt := time.Now().UTC().Add(sessionTTL)
	s.setSessionCookie(w, sessionID, expiresAt)
	http.Redirect(w, r, ticket.Next, http.StatusFound)
}

type ssoTicket struct {
	Subject     string
	Email       string
	DisplayName string
	AvatarURL   string
	JTI         string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	Next        string
}

var errInvalidSSOTicket = errors.New("invalid SSO ticket")

func verifySSOTicket(raw, secret, audience string, now time.Time) (ssoTicket, error) {
	if strings.TrimSpace(raw) == "" || len(raw) > 8192 || strings.TrimSpace(audience) == "" {
		return ssoTicket{}, errInvalidSSOTicket
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ssoTicket{}, errInvalidSSOTicket
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(provided, expected) {
		return ssoTicket{}, errInvalidSSOTicket
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payloadBytes) == 0 || len(payloadBytes) > 4096 {
		return ssoTicket{}, errInvalidSSOTicket
	}
	var payload map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(payloadBytes)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return ssoTicket{}, errInvalidSSOTicket
	}
	if stringValue(payload["iss"]) != "sub2api" || stringValue(payload["aud"]) != audience {
		return ssoTicket{}, errInvalidSSOTicket
	}
	subject := strings.TrimSpace(stringValue(payload["sub"]))
	jti := strings.TrimSpace(stringValue(payload["jti"]))
	issuedAt, okIssued := jsonUnixSeconds(payload["iat"])
	expiresAt, okExpires := jsonUnixSeconds(payload["exp"])
	if subject == "" || jti == "" || len(jti) > 256 || !okIssued || !okExpires || expiresAt <= issuedAt {
		return ssoTicket{}, errInvalidSSOTicket
	}
	nowUnix := now.Unix()
	if expiresAt <= nowUnix || issuedAt > nowUnix+30 || expiresAt-issuedAt > int64((2*time.Minute)/time.Second) {
		return ssoTicket{}, errInvalidSSOTicket
	}
	next := safeSSONext(stringValue(payload["next"]))
	return ssoTicket{
		Subject: subject, Email: stringValue(payload["email"]),
		DisplayName: firstNonEmpty(stringValue(payload["displayName"]), stringValue(payload["username"])),
		AvatarURL:   stringValue(payload["avatarUrl"]), JTI: jti,
		IssuedAt: time.Unix(issuedAt, 0).UTC(), ExpiresAt: time.Unix(expiresAt, 0).UTC(), Next: next,
	}, nil
}

func jsonUnixSeconds(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(typed.String(), 10, 64)
		return parsed, err == nil
	case float64:
		return int64(typed), typed == float64(int64(typed))
	case int64:
		return typed, true
	default:
		return 0, false
	}
}

func safeSSONext(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") {
		return "/"
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "/"
		}
	}
	return raw
}

func ticketUserJSON(ticket ssoTicket) ([]byte, error) {
	value := map[string]any{
		"id": ticket.Subject, "email": ticket.Email, "username": ticket.DisplayName,
		"display_name": ticket.DisplayName, "avatar_url": ticket.AvatarURL,
		"role": "user", "status": "active",
	}
	return json.Marshal(value)
}

func (s *Server) authRegister(w http.ResponseWriter, r *http.Request, requestID string) {
	if agent, err := s.store.Agent(s.cfg.AgentID); err != nil || agent.Status != "active" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "AGENT_SUSPENDED", "registration is disabled until agent configuration is ready")
		return
	}
	var payload map[string]any
	if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
		return
	}
	email := strings.TrimSpace(stringValue(payload["email"]))
	password := stringValue(payload["password"])
	if email == "" || password == "" {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "email and password are required")
		return
	}

	var auth MainAuthResult
	var err error
	if s.cfg.AdminKey != "" {
		// Provisioning-created users use the admin endpoint so registration does
		// not depend on a captcha/email worker being configured on the main site.
		// The browser still receives only the resulting AgentAPI cookie.
		adminPayload := map[string]any{
			"email": email, "password": password, "role": "user",
		}
		if username := strings.TrimSpace(stringValue(payload["username"])); username != "" {
			adminPayload["username"] = username
		}
		if _, err := s.main.AdminCreateUser(r.Context(), adminPayload); err != nil {
			var apiErr *MainAPIError
			if !errors.As(err, &apiErr) || (apiErr.Status != http.StatusConflict && !strings.Contains(strings.ToUpper(apiErr.Code), "EXISTS")) {
				s.writeMainError(w, requestID, err)
				return
			}
			// Existing account: let the normal registration endpoint decide whether
			// it can be reused; normally it returns a conflict.
		}
		auth, err = s.main.Login(r.Context(), email, password)
	} else {
		// Without the server-side admin credential, use the public registration
		// contract directly so captcha/email verification semantics are preserved.
		auth, err = s.main.Register(r.Context(), payload)
	}
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	if auth.Requires2FA {
		s.writeData(w, http.StatusOK, requestID, map[string]any{
			"requires_2fa": true, "temp_token": auth.TempToken,
		})
		return
	}
	s.establishSession(w, r, requestID, auth, true)
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request, requestID string) {
	var payload struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
		return
	}
	auth, err := s.main.Login(r.Context(), strings.TrimSpace(payload.Email), payload.Password)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	if auth.Requires2FA {
		s.writeData(w, http.StatusOK, requestID, map[string]any{
			"requires_2fa": true, "temp_token": auth.TempToken,
		})
		return
	}
	s.establishSession(w, r, requestID, auth, false)
}

func (s *Server) authLogin2FA(w http.ResponseWriter, r *http.Request, requestID string) {
	var payload struct {
		TempToken string `json:"temp_token"`
		Code      string `json:"totp_code"`
	}
	if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
		return
	}
	auth, err := s.main.Login2FA(r.Context(), payload.TempToken, payload.Code)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	s.establishSession(w, r, requestID, auth, false)
}

func (s *Server) establishSession(w http.ResponseWriter, r *http.Request, requestID string, auth MainAuthResult, createMapping bool) {
	if auth.AccessToken == "" {
		s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_AUTH_INVALID", "main site did not return an access token")
		return
	}
	userJSON := append(json.RawMessage(nil), auth.User...)
	if len(userJSON) == 0 || string(userJSON) == "null" || mainUserIDFromJSON(userJSON) == "" {
		current, err := s.main.CurrentUser(r.Context(), auth.AccessToken)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		userJSON = current
	}
	mainUserID := mainUserIDFromJSON(userJSON)
	if mainUserID == "" {
		s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_USER_INVALID", "main site user id is missing")
		return
	}
	email, displayName, _ := userJSONFields(userJSON)
	_, mapErr := s.store.User(s.cfg.AgentID, mainUserID)
	if errors.Is(mapErr, errNotFound) {
		allowed := createMapping || s.cfg.AutoBindExistingUsers || mainUserID == s.cfg.OwnerMainUserID
		if !allowed {
			s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_NOT_MAPPED", "this main-site account is not registered on this agent")
			return
		}
		if _, err := s.store.UpsertUser(s.cfg.AgentID, mainUserID, email, displayName); err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to create agent user mapping")
			return
		}
	} else if mapErr != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load agent user mapping")
		return
	}

	expiresAt := time.Now().UTC().Add(sessionTTL)
	sessionID, err := s.store.CreateSession(mainUserID, userJSON, auth.AccessToken, auth.RefreshToken, expiresAt)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "SESSION_CREATE_FAILED", "failed to create session")
		return
	}
	s.setSessionCookie(w, sessionID, expiresAt)
	s.writeData(w, http.StatusOK, requestID, s.browserAuthResponse(userJSON, mainUserID))
}

func (s *Server) authMe(w http.ResponseWriter, r *http.Request, requestID string) {
	session, user, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	current, err := s.currentSessionUser(r.Context(), session)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	if id := mainUserIDFromJSON(current); id != "" && id == session.MainUserID {
		if email, display, _ := userJSONFields(current); email != "" || display != "" {
			_, _ = s.store.UpsertUser(s.cfg.AgentID, session.MainUserID, email, display)
		}
	}
	_ = user
	s.writeData(w, http.StatusOK, requestID, s.browserUser(current, session.MainUserID))
}

func (s *Server) authRefresh(w http.ResponseWriter, r *http.Request, requestID string) {
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	if session.RefreshToken == "" {
		s.writeError(w, http.StatusUnauthorized, requestID, "REFRESH_TOKEN_UNAVAILABLE", "session cannot be refreshed")
		return
	}
	auth, err := s.main.Refresh(r.Context(), session.RefreshToken)
	if err != nil {
		_ = s.store.DeleteSession(session.ID)
		s.clearSessionCookie(w)
		s.writeMainError(w, requestID, err)
		return
	}
	current := auth.User
	if len(current) == 0 || mainUserIDFromJSON(current) == "" {
		current, err = s.main.CurrentUser(r.Context(), auth.AccessToken)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
	}
	if err := s.store.UpdateSession(session.ID, session.MainUserID, current, auth.AccessToken, auth.RefreshToken, time.Now().UTC().Add(sessionTTL)); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "SESSION_UPDATE_FAILED", "failed to refresh session")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"expires_in": int(sessionTTL.Seconds()), "token_type": "Cookie", "user": s.browserUser(current, session.MainUserID)})
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request, requestID string) {
	if cookie, err := r.Cookie(s.cfg.CookieName); err == nil {
		if session, loadErr := s.store.LoadSession(cookie.Value); loadErr == nil {
			_ = s.main.Logout(r.Context(), session.RefreshToken)
			_ = s.store.DeleteSession(cookie.Value)
		}
	}
	s.clearSessionCookie(w)
	s.writeData(w, http.StatusOK, requestID, map[string]any{"message": "Logged out successfully"})
}

func (s *Server) currentSessionUser(ctx context.Context, session Session) (json.RawMessage, error) {
	// SSO sessions intentionally contain no upstream access/refresh token. The
	// signed ticket already established the subject, and the model gateway uses
	// the server-side on-behalf-of header; do not fall back to any shared key.
	if strings.TrimSpace(session.AccessToken) == "" {
		if len(session.UserJSON) == 0 {
			return nil, &MainAPIError{Status: http.StatusUnauthorized, Code: "SESSION_IDENTITY_MISSING", Message: "session identity is missing"}
		}
		return append(json.RawMessage(nil), session.UserJSON...), nil
	}
	current, err := s.main.CurrentUser(ctx, session.AccessToken)
	if err == nil {
		return current, nil
	}
	var apiErr *MainAPIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized || session.RefreshToken == "" {
		return nil, err
	}
	auth, refreshErr := s.main.Refresh(ctx, session.RefreshToken)
	if refreshErr != nil {
		return nil, refreshErr
	}
	current, currentErr := s.main.CurrentUser(ctx, auth.AccessToken)
	if currentErr != nil {
		return nil, currentErr
	}
	_ = s.store.UpdateSession(session.ID, session.MainUserID, current, auth.AccessToken, auth.RefreshToken, time.Now().UTC().Add(sessionTTL))
	return current, nil
}

func (s *Server) handlePublicSettings(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	branding, err := s.store.Agent(s.cfg.AgentID)
	if err != nil && !errors.Is(err, errNotFound) {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load public branding")
		return
	}
	siteName := s.cfg.SiteName
	logo := s.cfg.SiteLogo
	if branding.SiteName != "" {
		siteName = branding.SiteName
	}
	if branding.SiteLogo != "" {
		logo = branding.SiteLogo
	}
	if logo == "" {
		logo = "/logo.svg"
	}
	settings := map[string]any{
		"registration_enabled": true, "email_verify_enabled": false,
		"force_email_on_third_party_signup": false, "registration_email_suffix_whitelist": []string{},
		"registration_email_domain_quota_enabled": false, "promo_code_enabled": false,
		"password_reset_enabled": false, "invitation_code_enabled": false,
		"login_agreement_enabled": false, "turnstile_enabled": false, "tencent_captcha_enabled": false,
		"passkey_enabled": false, "turnstile_site_key": "", "aliyun_captcha_enabled": false,
		"site_name": siteName, "site_logo": logo, "site_subtitle": "Agent API Gateway",
		"api_base_url": "/api/v1", "contact_info": "", "doc_url": "", "home_content": "",
		"compact_home_enabled": false, "hide_ccs_import_button": true,
		// The public flag only tells the UI whether this AgentAPI instance has
		// enabled its signed webhook bridge. It never exposes a secret or a main
		// site credential. A verified payment still requires synced owner credit.
		"payment_enabled": s.cfg.PaymentEnabled, "payment_provider": s.cfg.PaymentProvider,
		"payment_currency": s.cfg.PaymentCurrency, "payment_min_amount_cents": s.cfg.PaymentMinCents,
		"payment_max_amount_cents": s.cfg.PaymentMaxCents, "payment_balance_disabled": true,
		"risk_control_enabled": false, "table_default_page_size": 20, "table_page_size_options": []int{20, 50, 100},
		"custom_menu_items": []any{}, "custom_endpoints": []any{},
		"linuxdo_oauth_enabled": false, "wechat_oauth_enabled": false, "oidc_oauth_enabled": false,
		"oidc_oauth_provider_name": "", "github_oauth_enabled": false, "google_oauth_enabled": false,
		"backend_mode_enabled": false, "version": "agentapi", "balance_low_notify_enabled": false,
		"account_quota_notify_enabled": false, "balance_low_notify_threshold": 0,
		"channel_monitor_enabled": false, "channel_monitor_default_interval_seconds": 60,
		"available_channels_enabled": false, "subscription_enabled": false, "model_plaza_enabled": false,
		"model_plaza_require_auth": true, "plugin_management_enabled": false, "service_quota_enabled": false,
		"affiliate_enabled": false, "allow_user_view_error_requests": false,
	}
	s.writeData(w, http.StatusOK, requestID, settings)
}

func (s *Server) handleAgentContext(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	agent, err := s.store.Agent(s.cfg.AgentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load agent")
		return
	}
	// Branding and readiness are public; owner balance and allocation totals are
	// only visible to the configured agent administrator.
	publicAgent := redactAgentFinancials(agent)
	result := map[string]any{"agent": publicAgent, "authenticated": false, "is_agent_admin": false}
	if session, user, ok := s.loadSession(r); ok {
		result["authenticated"] = true
		result["is_agent_admin"] = s.isAgentAdmin(session)
		result["main_user_id"] = session.MainUserID
		result["user"] = user
		if s.isAgentAdmin(session) {
			result["agent"] = agent
		}
	}
	s.writeData(w, http.StatusOK, requestID, result)
}

func (s *Server) handleAgentWallet(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	session, user, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	agent, err := s.store.Wallet(s.cfg.AgentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load wallet")
		return
	}
	if !s.isAgentAdmin(session) {
		agent = redactAgentFinancials(agent)
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"agent": agent, "user": user, "main_user_id": session.MainUserID})
}

func (s *Server) handleAgentUsers(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	session, user, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	if s.isAgentAdmin(session) {
		users, err := s.store.Users(s.cfg.AgentID, 500)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to list agent users")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": users, "total": len(users)})
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"items": []AgentUserView{user}, "total": 1})
}

func (s *Server) handleAgentUsage(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	mainUserID := session.MainUserID
	if s.isAgentAdmin(session) {
		mainUserID = ""
	}
	items, err := s.store.Usage(s.cfg.AgentID, mainUserID, 500)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load usage")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items)})
}

// handleAgentRecharge owns the user-facing order lifecycle. It deliberately
// does not accept agent_id, main_user_id, provider, or currency from the
// browser: all of those values come from the authenticated session and the
// instance configuration.
func (s *Server) handleAgentRecharge(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if err := s.store.ExpireRechargeOrders(s.cfg.AgentID); err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to expire recharge orders")
			return
		}
		orders, err := s.store.RechargeOrders(s.cfg.AgentID, session.MainUserID, 100)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load recharge orders")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{
			"enabled":          s.cfg.PaymentEnabled,
			"provider":         s.cfg.PaymentProvider,
			"currency":         s.cfg.PaymentCurrency,
			"min_amount_cents": s.cfg.PaymentMinCents,
			"max_amount_cents": s.cfg.PaymentMaxCents,
			"items":            orders,
			"total":            len(orders),
		})
		return
	case http.MethodPost:
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		if !s.cfg.PaymentEnabled {
			s.writeError(w, http.StatusGone, requestID, "PAYMENT_DISABLED", "recharge is not enabled for this AgentAPI instance")
			return
		}
		var payload map[string]any
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
			return
		}
		amountCents, err := amountFromPayload(payload)
		if err != nil || amountCents < s.cfg.PaymentMinCents || amountCents > s.cfg.PaymentMaxCents {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_AMOUNT", fmt.Sprintf("amount must be between %s and %s %s", formatCents(s.cfg.PaymentMinCents), formatCents(s.cfg.PaymentMaxCents), s.cfg.PaymentCurrency))
			return
		}
		requestKey := requestIDFrom(r, payload)
		provider := strings.TrimSpace(s.cfg.PaymentProvider)
		currency := strings.ToUpper(strings.TrimSpace(s.cfg.PaymentCurrency))
		expiresAt := time.Now().UTC().Add(s.cfg.PaymentOrderTTL)
		order, created, err := s.store.CreateRechargeOrder(s.cfg.AgentID, session.MainUserID, amountCents, currency, provider, "", requestKey, expiresAt)
		if err != nil {
			status := http.StatusInternalServerError
			reason := "RECHARGE_ORDER_CREATE_FAILED"
			if errors.Is(err, errIdempotencyConflict) {
				status = http.StatusConflict
				reason = "IDEMPOTENCY_CONFLICT"
			} else if errors.Is(err, errNotFound) {
				status = http.StatusNotFound
				reason = "AGENT_USER_NOT_FOUND"
			}
			s.writeError(w, status, requestID, reason, "failed to create recharge order")
			return
		}
		if created && strings.TrimSpace(s.cfg.PaymentCheckoutURLTemplate) != "" {
			checkoutURL := paymentCheckoutURL(s.cfg.PaymentCheckoutURLTemplate, order)
			if updated, updateErr := s.store.SetRechargePaymentURL(s.cfg.AgentID, order.OrderNo, checkoutURL); updateErr == nil {
				order = updated
			} else {
				slog.Error("failed to persist payment checkout URL", "order_no", order.OrderNo, "error", updateErr)
			}
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		s.writeData(w, status, requestID, map[string]any{
			"order":   order,
			"enabled": true,
			"message": "complete payment with the configured provider; credit is allocated only after a verified webhook and owner-balance synchronization",
		})
		return
	default:
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

// handlePaymentWebhook implements the provider-neutral bridge. The endpoint
// is intentionally independent of browser sessions and never credits a wallet
// directly. A valid paid event only moves the order to
// paid_pending_allocation; allocation consumes already-synchronized owner
// credit in the same local transaction.
func (s *Server) handlePaymentWebhook(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "payment webhook requires POST")
		return
	}
	if !s.cfg.PaymentEnabled {
		s.writeError(w, http.StatusNotFound, requestID, "PAYMENT_DISABLED", "payment webhook is disabled")
		return
	}
	secret := strings.TrimSpace(s.cfg.PaymentWebhookSecret)
	if secret == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "PAYMENT_WEBHOOK_NOT_CONFIGURED", "payment webhook secret is not configured")
		return
	}
	eventID := strings.TrimSpace(r.Header.Get("X-Agent-Payment-Event-ID"))
	if eventID == "" || len(eventID) > 256 {
		s.writeError(w, http.StatusBadRequest, requestID, "PAYMENT_EVENT_ID_REQUIRED", "X-Agent-Payment-Event-ID is required")
		return
	}
	if timestamp := strings.TrimSpace(r.Header.Get("X-Agent-Payment-Timestamp")); timestamp != "" {
		if !validPaymentTimestamp(timestamp, time.Now().UTC()) {
			s.writeError(w, http.StatusUnauthorized, requestID, "PAYMENT_TIMESTAMP_INVALID", "payment webhook timestamp is invalid or expired")
			return
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBody+1))
	if err != nil || int64(len(body)) > maxJSONBody {
		s.writeError(w, http.StatusRequestEntityTooLarge, requestID, "REQUEST_TOO_LARGE", "payment webhook body is too large")
		return
	}
	if !verifyPaymentSignature(r.Header.Get("X-Agent-Payment-Signature"), secret, body) {
		s.writeError(w, http.StatusUnauthorized, requestID, "PAYMENT_SIGNATURE_INVALID", "payment webhook signature is invalid")
		return
	}
	var payload struct {
		OrderNo         string `json:"order_no"`
		Status          string `json:"status"`
		AmountCents     int64  `json:"amount_cents"`
		Currency        string `json:"currency"`
		ProviderTradeNo string `json:"provider_trade_no"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "payment webhook body must be valid JSON")
		return
	}
	payload.OrderNo = strings.TrimSpace(payload.OrderNo)
	payload.Status = strings.ToLower(strings.TrimSpace(payload.Status))
	payload.Currency = strings.ToUpper(strings.TrimSpace(payload.Currency))
	payload.ProviderTradeNo = strings.TrimSpace(payload.ProviderTradeNo)
	if payload.OrderNo == "" || payload.AmountCents <= 0 || payload.Currency == "" || payload.ProviderTradeNo == "" || len(payload.ProviderTradeNo) > 256 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PAYMENT_EVENT", "order_no, status, amount_cents, currency and provider_trade_no are required")
		return
	}
	if err := s.store.ExpireRechargeOrders(s.cfg.AgentID); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to expire recharge orders")
		return
	}
	payloadHash := sha256.Sum256(body)
	result, err := s.store.RecordPaymentEvent(s.cfg.AgentID, s.cfg.PaymentProvider, eventID, payload.OrderNo, payload.Status, payload.AmountCents, payload.Currency, payload.ProviderTradeNo, hex.EncodeToString(payloadHash[:]))
	if err != nil {
		status := http.StatusBadRequest
		reason := "PAYMENT_EVENT_REJECTED"
		if errors.Is(err, errNotFound) {
			status = http.StatusNotFound
			reason = "RECHARGE_ORDER_NOT_FOUND"
		} else if errors.Is(err, errIdempotencyConflict) || errors.Is(err, errRechargeStateConflict) || errors.Is(err, errRechargeExpired) {
			status = http.StatusConflict
			reason = "PAYMENT_EVENT_CONFLICT"
		}
		s.writeError(w, status, requestID, reason, err.Error())
		return
	}

	order := result.Order
	allocated := order.Status == "allocated"
	allocationPending := order.Status == "paid_pending_allocation"
	if allocationPending {
		if allocatedOrder, allocateErr := s.syncAndAllocateRechargeOrder(r.Context(), requestID+":payment-sync", order.OrderNo); allocateErr != nil {
			if errors.Is(allocateErr, errInsufficientBalance) {
				s.writeData(w, http.StatusAccepted, requestID, map[string]any{"order": order, "allocated": false, "allocation_pending": true, "retryable": true})
				return
			}
			// The event is durable. Returning 202 tells a provider that the
			// payload was accepted while the operator/reconciler can retry the
			// allocation after the main site is reachable.
			s.writeData(w, http.StatusAccepted, requestID, map[string]any{"order": order, "allocated": false, "allocation_pending": true, "retryable": true})
			return
		} else {
			order = allocatedOrder
			allocated = order.Status == "allocated"
		}
	}
	status := http.StatusOK
	if allocationPending && !allocated {
		status = http.StatusAccepted
	}
	s.writeData(w, status, requestID, map[string]any{"order": order, "allocated": allocated, "allocation_pending": !allocated && order.Status == "paid_pending_allocation", "duplicate": !result.Created})
}

func (s *Server) paymentAdminOrders(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if err := s.store.ExpireRechargeOrders(s.cfg.AgentID); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to expire recharge orders")
		return
	}
	orders, err := s.store.RechargeOrders(s.cfg.AgentID, "", 500)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load recharge orders")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"enabled": s.cfg.PaymentEnabled, "items": orders, "total": len(orders)})
}

func (s *Server) paymentAdminAllocate(w http.ResponseWriter, r *http.Request, requestID, actorID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	var payload struct {
		OrderNo string `json:"order_no"`
	}
	if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
		return
	}
	orderNo := strings.TrimSpace(payload.OrderNo)
	if orderNo == "" {
		s.writeError(w, http.StatusBadRequest, requestID, "ORDER_NO_REQUIRED", "order_no is required")
		return
	}
	if err := s.store.ExpireRechargeOrders(s.cfg.AgentID); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to expire recharge orders")
		return
	}
	order, err := s.store.RechargeOrder(s.cfg.AgentID, orderNo)
	if err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "RECHARGE_ORDER_NOT_FOUND", "recharge order not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load recharge order")
		return
	}
	if order.Status == "allocated" {
		s.recordAudit("agent_admin", actorID, "recharge.allocate", "recharge_order", orderNo, requestID, "success", "idempotent replay")
		s.writeData(w, http.StatusOK, requestID, map[string]any{"order": order, "allocated": true})
		return
	}
	if order.Status != "paid_pending_allocation" {
		s.writeError(w, http.StatusConflict, requestID, "RECHARGE_NOT_PAID", "only paid_pending_allocation orders can be allocated")
		return
	}
	order, err = s.syncAndAllocateRechargeOrder(r.Context(), requestID+":payment-admin-sync", orderNo)
	if err != nil {
		if errors.Is(err, errInsufficientBalance) {
			s.writeError(w, http.StatusPaymentRequired, requestID, "MAIN_OWNER_BALANCE_INSUFFICIENT", "owner balance is not sufficient for this paid order")
			return
		}
		var mainErr *MainAPIError
		if errors.As(err, &mainErr) {
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "RECHARGE_ALLOCATION_FAILED", "failed to allocate paid recharge order")
		return
	}
	s.recordAudit("agent_admin", actorID, "recharge.allocate", "recharge_order", orderNo, requestID, "success", "")
	s.writeData(w, http.StatusOK, requestID, map[string]any{"order": order, "allocated": order.Status == "allocated"})
}

func paymentCheckoutURL(template string, order RechargeOrder) string {
	return strings.NewReplacer(
		"{order_no}", url.QueryEscape(order.OrderNo),
		"{amount}", formatCents(order.AmountCents),
		"{amount_cents}", strconv.FormatInt(order.AmountCents, 10),
		"{currency}", url.QueryEscape(order.Currency),
	).Replace(template)
}

func formatCents(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	value := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if negative {
		return "-" + value
	}
	return value
}

func validateBrandingText(value, field string, maxLength int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(value) > maxLength {
		return fmt.Errorf("%s must be at most %d bytes", field, maxLength)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains a control character", field)
		}
	}
	return nil
}

func validateBrandLogo(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 2048 {
		return fmt.Errorf("site_logo must be at most 2048 bytes")
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("site_logo contains a control character")
		}
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("site_logo must be an https/http URL or an application-relative path")
	}
	return nil
}

func verifyPaymentSignature(rawSignature, secret string, body []byte) bool {
	signature := strings.TrimSpace(rawSignature)
	if !strings.HasPrefix(strings.ToLower(signature), "sha256=") {
		return false
	}
	encoded := strings.TrimSpace(signature[len("sha256="):])
	provided, err := hex.DecodeString(encoded)
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

func validPaymentTimestamp(raw string, now time.Time) bool {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return false
	}
	stamp := time.Unix(value, 0)
	return stamp.After(now.Add(-10*time.Minute)) && stamp.Before(now.Add(10*time.Minute))
}

func (s *Server) handleAgentAdmin(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.URL.Path == "/api/v1/agent/admin/wallet/credit" {
		// There is intentionally no local top-up endpoint in owner_upstream mode.
		// Main-site payments change the owner's balance; AgentAPI can only read
		// that balance through the server-side administrator credential.
		s.writeError(w, http.StatusGone, requestID, "LOCAL_CREDIT_DISABLED", "agent balance is funded on the main site")
		return
	}

	session, _, ok := s.requireSession(w, r, requestID)
	if !ok || !s.isAgentAdmin(session) {
		if ok {
			s.writeError(w, http.StatusForbidden, requestID, "AGENT_ADMIN_REQUIRED", "agent administrator access required")
		}
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/branding" {
		switch r.Method {
		case http.MethodGet:
			agent, err := s.store.Agent(s.cfg.AgentID)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load branding")
				return
			}
			s.writeData(w, http.StatusOK, requestID, map[string]any{
				"name": agent.Name, "site_name": agent.SiteName, "site_logo": agent.SiteLogo,
			})
			return
		case http.MethodPut, http.MethodPatch:
			if !sameOrigin(r) {
				s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
				return
			}
			var payload struct {
				Name     *string `json:"name"`
				SiteName *string `json:"site_name"`
				SiteLogo *string `json:"site_logo"`
			}
			if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
				return
			}
			current, err := s.store.Agent(s.cfg.AgentID)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load branding")
				return
			}
			name, siteName, siteLogo := current.Name, current.SiteName, current.SiteLogo
			if payload.Name != nil {
				name = strings.TrimSpace(*payload.Name)
			}
			if payload.SiteName != nil {
				siteName = strings.TrimSpace(*payload.SiteName)
			}
			if payload.SiteLogo != nil {
				siteLogo = strings.TrimSpace(*payload.SiteLogo)
			}
			if err := validateBrandingText(name, "name", 100); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BRANDING", err.Error())
				return
			}
			if err := validateBrandingText(siteName, "site_name", 160); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BRANDING", err.Error())
				return
			}
			if err := validateBrandLogo(siteLogo); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BRANDING", err.Error())
				return
			}
			updated, err := s.store.UpdateBranding(s.cfg.AgentID, name, siteName, siteLogo)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "BRANDING_UPDATE_FAILED", "failed to save branding")
				return
			}
			s.recordAudit("agent_admin", session.MainUserID, "branding.update", "agent", s.cfg.AgentID, requestID, "success", "")
			s.writeData(w, http.StatusOK, requestID, map[string]any{
				"name": updated.Name, "site_name": updated.SiteName, "site_logo": updated.SiteLogo,
			})
			return
		default:
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported branding operation")
			return
		}
	}
	if r.URL.Path == "/api/v1/agent/admin/audit-events" && r.Method == http.MethodGet {
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
				limit = parsed
			}
		}
		items, err := s.store.AuditEvents(s.cfg.AgentID, limit)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load audit events")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items)})
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/wallet" && r.Method == http.MethodGet {
		agent, err := s.store.Agent(s.cfg.AgentID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load wallet")
			return
		}
		s.writeData(w, http.StatusOK, requestID, agent)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/recharge/orders" {
		s.paymentAdminOrders(w, r, requestID)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/recharge/allocate" {
		s.paymentAdminAllocate(w, r, requestID, session.MainUserID)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/wallet/sync" && r.Method == http.MethodPost {
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		agent, err := s.syncOwnerBalance(r.Context(), requestID)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.recordAudit("agent_admin", session.MainUserID, "wallet.sync", "agent_wallet", s.cfg.AgentID, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, agent)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/settlements" && r.Method == http.MethodGet {
		items, err := s.store.PendingSettlements(s.cfg.AgentID, s.cfg.SettlementReconcileBatch)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load pending settlements")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items)})
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/settlements/reconcile" && r.Method == http.MethodPost {
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		var payload struct {
			RequestID string `json:"request_id"`
		}
		if _, err := decodeOptionalJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
			return
		}
		requestIDFilter := strings.TrimSpace(payload.RequestID)
		if requestIDFilter == "" {
			requestIDFilter = strings.TrimSpace(r.URL.Query().Get("request_id"))
		}
		items, err := s.reconcileSettlements(r.Context(), requestIDFilter)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.recordAudit("agent_admin", session.MainUserID, "settlements.reconcile", "settlement", requestIDFilter, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items)})
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/users" && r.Method == http.MethodGet {
		users, err := s.store.Users(s.cfg.AgentID, 500)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to list users")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": users, "total": len(users)})
		return
	}
	const prefix = "/api/v1/agent/admin/users/"
	if strings.HasPrefix(r.URL.Path, prefix) && strings.HasSuffix(r.URL.Path, "/allocate") && r.Method == http.MethodPost {
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		mainUserID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/allocate")
		mainUserID, _ = url.PathUnescape(mainUserID)
		if mainUserID == "" {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_USER", "main user id is required")
			return
		}
		var payload map[string]any
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
			return
		}
		amount, err := amountFromPayload(payload)
		if err != nil || amount <= 0 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_AMOUNT", "a positive amount is required")
			return
		}
		if _, err := s.store.User(s.cfg.AgentID, mainUserID); err != nil {
			s.writeError(w, http.StatusNotFound, requestID, "AGENT_USER_NOT_FOUND", "user is not mapped to this agent")
			return
		}
		user, err := s.syncAndAllocateUser(r.Context(), requestID, mainUserID, amount, requestIDFrom(r, payload), stringValue(payload["order_id"]), stringValue(payload["note"]))
		if err != nil {
			var mainErr *MainAPIError
			if errors.As(err, &mainErr) {
				s.writeMainError(w, requestID, err)
				return
			}
			if errors.Is(err, errNotFound) {
				s.writeError(w, http.StatusNotFound, requestID, "AGENT_USER_NOT_FOUND", "user is not mapped to this agent")
				return
			}
			status := http.StatusConflict
			reason := "ALLOCATION_FAILED"
			message := "failed to allocate agent credit"
			if errors.Is(err, errInsufficientBalance) {
				status = http.StatusPaymentRequired
				reason = "AGENT_INSUFFICIENT_BALANCE"
				message = err.Error()
			} else if errors.Is(err, errIdempotencyConflict) {
				reason = "IDEMPOTENCY_CONFLICT"
				message = "allocation idempotency key is already associated with a different operation"
			}
			s.writeError(w, status, requestID, reason, message)
			return
		}
		s.recordAudit("agent_admin", session.MainUserID, "wallet.allocate", "agent_user", mainUserID, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, user)
		return
	}
	s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "agent admin endpoint not found")
}

// handleAPIKeys manages AgentAPI-local gateway credentials. These keys are
// deliberately independent from Sub2API's user keys: the full value is
// returned once at creation time, only a hash is persisted, and the key is
// never forwarded to the main site. All management operations require the
// browser's opaque AgentAPI session; a bearer key cannot create or revoke a
// sibling key.
func (s *Server) handleAPIKeys(w http.ResponseWriter, r *http.Request, requestID string) {
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}

	if r.URL.Path == "/api/v1/api-keys" && r.Method == http.MethodGet {
		keys, err := s.store.APIKeys(s.cfg.AgentID, session.MainUserID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to list AgentAPI keys")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": keys, "total": len(keys)})
		return
	}

	if r.URL.Path == "/api/v1/api-keys" && r.Method == http.MethodPost {
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		var payload struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
			return
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			name = "AgentAPI key"
		}
		if len(name) > 100 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_NAME", "key name must be at most 100 characters")
			return
		}
		view, raw, err := s.store.CreateAPIKey(s.cfg.AgentID, session.MainUserID, name)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "API_KEY_CREATE_FAILED", "failed to create AgentAPI key")
			return
		}
		s.recordAudit("agent_user", session.MainUserID, "api_key.create", "agent_api_key", strconv.FormatInt(view.ID, 10), requestID, "success", "")
		s.writeData(w, http.StatusCreated, requestID, map[string]any{"item": view, "key": raw})
		return
	}

	const prefix = "/api/v1/api-keys/"
	if strings.HasPrefix(r.URL.Path, prefix) && r.Method == http.MethodDelete {
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		idRaw := strings.TrimPrefix(r.URL.Path, prefix)
		if idRaw == "" || strings.Contains(idRaw, "/") {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_KEY_ID", "key id is required")
			return
		}
		id, err := strconv.ParseInt(idRaw, 10, 64)
		if err != nil || id <= 0 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_KEY_ID", "key id is invalid")
			return
		}
		if err := s.store.RevokeAPIKey(s.cfg.AgentID, session.MainUserID, id); err != nil {
			if errors.Is(err, errNotFound) {
				s.writeError(w, http.StatusNotFound, requestID, "API_KEY_NOT_FOUND", "AgentAPI key not found")
				return
			}
			s.writeError(w, http.StatusInternalServerError, requestID, "API_KEY_REVOKE_FAILED", "failed to revoke AgentAPI key")
			return
		}
		s.recordAudit("agent_user", session.MainUserID, "api_key.revoke", "agent_api_key", strconv.FormatInt(id, 10), requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, map[string]any{"id": id, "status": "revoked"})
		return
	}

	s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported AgentAPI key operation")
}

func (s *Server) handleMainAPIProxy(w http.ResponseWriter, r *http.Request, requestID string) {
	if !isAllowedUserAPIPath(r.URL.Path, r.Method) {
		s.writeError(w, http.StatusForbidden, requestID, "AGENT_ROUTE_FORBIDDEN", "this main-site route is not exposed by AgentAPI")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxModelBody))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "failed to read request body")
		return
	}
	status, headers, responseBody, updated, err := s.proxyUserAPI(r, session, body, requestID)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	if updated != nil {
		session = *updated
		_ = session
	}
	copyResponse(w, status, headers, responseBody)
}

func (s *Server) proxyUserAPI(r *http.Request, session Session, body []byte, requestID string) (int, http.Header, []byte, *Session, error) {
	try := func(current Session) (int, http.Header, []byte, error) {
		pathSuffix := strings.TrimPrefix(r.URL.Path, "/api/v1")
		target := s.main.endpoint(pathSuffix)
		if query := r.URL.Query().Encode(); query != "" {
			target += "?" + query
		}
		headers := make(http.Header)
		for _, key := range []string{"Accept", "Content-Type", "Accept-Language", "User-Agent"} {
			if value := r.Header.Get(key); value != "" {
				headers.Set(key, value)
			}
		}
		headers.Set("Authorization", "Bearer "+current.AccessToken)
		headers.Set("X-Request-ID", requestID)
		resp, data, err := s.main.request(r.Context(), r.Method, target, body, headers)
		if err != nil {
			return 0, nil, nil, err
		}
		return resp.StatusCode, filteredResponseHeaders(resp.Header), data, nil
	}
	status, headers, responseBody, err := try(session)
	if err == nil && status != http.StatusUnauthorized {
		return status, headers, responseBody, nil, nil
	}
	if status != http.StatusUnauthorized || session.RefreshToken == "" {
		return status, headers, responseBody, nil, err
	}
	auth, refreshErr := s.main.Refresh(r.Context(), session.RefreshToken)
	if refreshErr != nil {
		return status, headers, responseBody, nil, refreshErr
	}
	current, currentErr := s.main.CurrentUser(r.Context(), auth.AccessToken)
	if currentErr != nil {
		return status, headers, responseBody, nil, currentErr
	}
	updated := session
	updated.AccessToken, updated.RefreshToken = auth.AccessToken, auth.RefreshToken
	updated.UserJSON = current
	updated.ExpiresAt = time.Now().UTC().Add(sessionTTL)
	_ = s.store.UpdateSession(updated.ID, updated.MainUserID, updated.UserJSON, updated.AccessToken, updated.RefreshToken, updated.ExpiresAt)
	status, headers, responseBody, err = try(updated)
	return status, headers, responseBody, &updated, err
}

func (s *Server) handleModelRelay(w http.ResponseWriter, r *http.Request, requestID string) {
	if agent, err := s.store.Agent(s.cfg.AgentID); err != nil || agent.Status != "active" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "AGENT_SUSPENDED", "agent is not active for model requests")
		return
	}
	if !isAllowedModelPath(r.URL.Path, r.Method) {
		s.writeError(w, http.StatusForbidden, requestID, "MODEL_ROUTE_FORBIDDEN", "this model route is not exposed by AgentAPI")
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "model relay supports GET and POST")
		return
	}
	principal, ok := s.requireModelPrincipal(w, r, requestID)
	if !ok {
		return
	}
	if r.URL.Path == "/v1/models" {
		// Do not proxy the main site's complete model inventory. It may contain
		// private/provider-specific names that are outside the satellite public
		// catalog and would let clients discover an unintended route.
		s.writePublicModels(w)
		return
	}
	if r.Method == http.MethodGet {
		if strings.HasPrefix(r.URL.Path, "/v1/videos/") {
			s.handleVideoPoll(w, r, requestID, principal)
			return
		}
		status, headers, data, err := s.main.RelayModelMethod(r.Context(), r.Method, r.URL.Path, r.URL.Query(), nil, s.cfg.OwnerMainUserID, requestID)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		copyResponse(w, status, headers, data)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxModelBody+1))
	if err != nil || int64(len(body)) > maxModelBody {
		s.writeError(w, http.StatusRequestEntityTooLarge, requestID, "REQUEST_TOO_LARGE", "model request body is too large")
		return
	}
	if err := validatePublicModel(r.Header.Get("Content-Type"), body); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "MODEL_NOT_ALLOWED", err.Error())
		return
	}
	if principal.User.Status != "active" {
		s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_DISABLED", "agent user is disabled")
		return
	}
	chargeID := requestIDFrom(r, nil)
	ownerID := strings.TrimSpace(s.cfg.OwnerMainUserID)
	if ownerID == "" || strings.TrimSpace(s.cfg.AdminKey) == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "BILLING_NOT_READY", "main billing owner is not configured")
		return
	}
	// Serialize the before/relay/after balance observation for one owner. This
	// prevents two concurrent requests from attributing the same asynchronous
	// balance delta to both proxy users.
	ownerMu := s.userSettlementMutex(ownerID)
	ownerMu.Lock()
	defer ownerMu.Unlock()
	// The settlement row is the durable request idempotency record. A retry
	// must never send the same model operation to the main site a second time,
	// even if the first process died after the upstream call.
	if existing, lookupErr := s.store.Settlement(s.cfg.AgentID, chargeID); lookupErr == nil {
		s.writeSettlementReplay(w, requestID, existing)
		return
	} else if !errors.Is(lookupErr, errNotFound) {
		s.writeError(w, http.StatusInternalServerError, requestID, "SETTLEMENT_LOOKUP_FAILED", "failed to load request settlement")
		return
	}
	before, ok := s.readMainBalance(r.Context(), ownerID)
	if !ok {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "BILLING_BALANCE_UNAVAILABLE", "main owner balance could not be read")
		return
	}
	agent, syncErr := s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, before, "balance-before:"+chargeID)
	if syncErr != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "BILLING_SNAPSHOT_FAILED", "failed to persist owner balance snapshot")
		return
	}
	if before < agent.WalletAllocated {
		s.writeError(w, http.StatusPaymentRequired, requestID, "MAIN_OWNER_BALANCE_OVERALLOCATED", "main owner balance is below allocated agent credit")
		return
	}
	if before < s.cfg.MaxRequestCostCents {
		s.writeError(w, http.StatusPaymentRequired, requestID, "MAIN_OWNER_BALANCE_INSUFFICIENT", "main owner balance is insufficient")
		return
	}
	settlement, created, err := s.store.PrepareSettlement(s.cfg.AgentID, principal.ProxyMainUserID, ownerID, chargeID, chargeID, s.cfg.MaxRequestCostCents)
	if err != nil {
		if errors.Is(err, errInsufficientBalance) {
			s.writeError(w, http.StatusPaymentRequired, requestID, "AGENT_INSUFFICIENT_BALANCE", "agent user balance is insufficient")
			return
		}
		if errors.Is(err, errIdempotencyConflict) {
			s.writeError(w, http.StatusConflict, requestID, "IDEMPOTENCY_CONFLICT", "request id is already associated with a different settlement")
			return
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "SETTLEMENT_PREPARE_FAILED", "failed to prepare billing settlement")
		return
	}
	if !created {
		s.writeSettlementReplay(w, requestID, settlement)
		return
	}

	if modelRequestWantsStream(r.Header.Get("Accept"), r.Header.Get("Content-Type"), body) {
		s.relayStreamingModel(w, r, requestID, ownerID, chargeID, before, body, r.Header.Get("Content-Type"))
		return
	}

	status, headers, responseBody, relayErr := s.main.RelayModelMethodWithContentType(r.Context(), r.Method, r.URL.Path, r.URL.Query(), body, r.Header.Get("Content-Type"), ownerID, chargeID)
	if relayErr != nil {
		// A transport failure does not tell us whether Sub2API received and
		// charged the request. Keep the reservation and reconcile it later.
		if err := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, relayErr.Error()); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "request outcome is uncertain and local settlement could not be recorded")
			return
		}
		s.writeMainError(w, requestID, relayErr)
		return
	}
	if status < 200 || status >= 300 {
		if status == http.StatusTooManyRequests || status >= 500 {
			if err := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, fmt.Sprintf("upstream status %d; charge is uncertain", status)); err != nil {
				s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "request outcome is uncertain and local settlement could not be recorded")
				return
			}
			copyResponse(w, status, headers, responseBody)
			return
		}
		if err := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "released", 0, fmt.Sprintf("upstream status %d", status)); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream rejection was received but local reservation could not be released")
			return
		}
		copyResponse(w, status, headers, responseBody)
		return
	}
	if r.URL.Path == "/v1/videos" {
		taskID, taskStatus := videoTaskIdentity(responseBody)
		if taskID == "" {
			_ = s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, "video creation response did not contain a task id")
			s.writeJSON(w, http.StatusBadGateway, apiResponse{Code: http.StatusBadGateway, Message: "video task was created but its id could not be tracked", Reason: "VIDEO_TASK_ID_MISSING", RequestID: requestID})
			return
		}
		if _, err := s.store.RecordVideoTask(s.cfg.AgentID, principal.ProxyMainUserID, taskID, chargeID, taskStatus); err != nil {
			_ = s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, "video task mapping could not be persisted")
			s.writeJSON(w, http.StatusServiceUnavailable, apiResponse{Code: http.StatusServiceUnavailable, Message: "video task was created but local tracking failed", Reason: "VIDEO_TASK_TRACKING_FAILED", RequestID: requestID, Data: map[string]string{"task_id": taskID}})
			return
		}
	}

	after, hasAfter := s.readMainBalance(r.Context(), ownerID)
	if !hasAfter {
		if err := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, "owner balance could not be read after relay"); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream succeeded but local settlement could not be recorded")
			return
		}
		copyResponse(w, status, headers, responseBody)
		return
	}
	delta := before - after
	if delta < 0 {
		delta = 0
	}
	if delta == 0 {
		// Upstream usage can be asynchronous. A zero delta is not permission to
		// refund; a reconciler must confirm the final charge first.
		if err := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, "owner usage is not visible yet"); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream succeeded but local settlement could not be recorded")
			return
		}
		_, _ = s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, after, "balance-pending:"+chargeID)
		copyResponse(w, status, headers, responseBody)
		return
	}
	if delta > s.cfg.MaxRequestCostCents {
		if err := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", delta, "actual owner charge exceeds local reservation"); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream charge exceeds reservation and local settlement could not be recorded")
			return
		}
		_, _ = s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, after, "balance-pending:"+chargeID)
		copyResponse(w, status, headers, responseBody)
		return
	}
	if err := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "confirmed", delta, ""); err != nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream succeeded but local settlement could not be finalized")
		return
	}
	_, _ = s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, after, "balance-after:"+chargeID)
	copyResponse(w, status, headers, responseBody)
}

// relayStreamingModel forwards an explicitly streaming request while keeping
// the local reservation until EOF. If the client disconnects or the upstream
// transport fails, the reservation remains pending for usage reconciliation;
// it is never guessed or silently refunded after partial output.
func (s *Server) handleVideoPoll(w http.ResponseWriter, r *http.Request, requestID string, principal modelPrincipal) {
	taskID := strings.TrimPrefix(r.URL.Path, "/v1/videos/")
	task, err := s.store.VideoTask(s.cfg.AgentID, taskID)
	if err != nil || task.MainUserID != principal.ProxyMainUserID {
		s.writeError(w, http.StatusNotFound, requestID, "VIDEO_TASK_NOT_FOUND", "video task was not found for this agent user")
		return
	}
	ownerID := strings.TrimSpace(s.cfg.OwnerMainUserID)
	if ownerID == "" || strings.TrimSpace(s.cfg.AdminKey) == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "BILLING_NOT_READY", "main billing owner is not configured")
		return
	}
	ownerMu := s.userSettlementMutex(ownerID)
	ownerMu.Lock()
	defer ownerMu.Unlock()
	before, hasBefore := s.readMainBalance(r.Context(), ownerID)
	status, headers, data, relayErr := s.main.RelayModelMethod(r.Context(), http.MethodGet, r.URL.Path, r.URL.Query(), nil, ownerID, task.RequestID)
	if relayErr != nil {
		s.writeMainError(w, requestID, relayErr)
		return
	}
	if status >= 200 && status < 300 {
		_, upstreamStatus := videoTaskIdentity(data)
		if upstreamStatus != "" {
			_ = s.store.UpdateVideoTaskStatus(s.cfg.AgentID, taskID, upstreamStatus)
		}
		if settlement, lookupErr := s.store.Settlement(s.cfg.AgentID, task.RequestID); lookupErr == nil && settlement.Status == "pending" {
			after, hasAfter := s.readMainBalance(r.Context(), ownerID)
			if hasBefore && hasAfter {
				delta := before - after
				if delta < 0 {
					delta = 0
				}
				switch {
				case delta > settlement.ReservedCents:
					_ = s.store.FinalizeSettlement(s.cfg.AgentID, task.RequestID, task.RequestID, "pending", delta, "video charge exceeds local reservation")
				case delta > 0:
					_ = s.store.FinalizeSettlement(s.cfg.AgentID, task.RequestID, task.RequestID, "confirmed", delta, "")
					_, _ = s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, after, "balance-video:"+task.RequestID)
				case videoTaskFailed(upstreamStatus):
					_ = s.store.FinalizeSettlement(s.cfg.AgentID, task.RequestID, task.RequestID, "released", 0, "video task ended without a charge: "+upstreamStatus)
				}
			}
		}
	}
	copyResponse(w, status, headers, data)
}

func videoTaskIdentity(data []byte) (string, string) {
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return "", ""
	}
	read := func(value map[string]any) (string, string) {
		id := firstNonEmpty(stringValue(value["id"]), stringValue(value["task_id"]), stringValue(value["video_id"]))
		status := strings.ToLower(firstNonEmpty(stringValue(value["status"]), stringValue(value["state"])))
		return id, status
	}
	id, status := read(payload)
	if nested, ok := payload["data"].(map[string]any); ok {
		nestedID, nestedStatus := read(nested)
		if id == "" {
			id = nestedID
		}
		if status == "" {
			status = nestedStatus
		}
	}
	if status == "" {
		status = "pending"
	}
	return id, status
}

func videoTaskFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "cancelled", "canceled", "rejected", "expired":
		return true
	default:
		return false
	}
}

func (s *Server) relayStreamingModel(w http.ResponseWriter, r *http.Request, requestID, ownerID, chargeID string, before int64, body []byte, contentType string) {
	resp, err := s.main.OpenModelResponse(r.Context(), r.Method, r.URL.Path, r.URL.Query(), body, contentType, ownerID, chargeID)
	if err != nil {
		if finalizeErr := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, err.Error()); finalizeErr != nil {
			slog.Error("failed to persist streaming pending settlement", "request_id", requestID, "error", finalizeErr)
			return
		}
		s.writeMainError(w, requestID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		if readErr != nil {
			_ = s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, readErr.Error())
			s.writeError(w, http.StatusServiceUnavailable, requestID, "UPSTREAM_RESPONSE_UNREADABLE", "upstream response could not be read")
			return
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if finalizeErr := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, fmt.Sprintf("upstream status %d; charge is uncertain", resp.StatusCode)); finalizeErr != nil {
				s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "request outcome is uncertain and local settlement could not be recorded")
				return
			}
			copyResponse(w, resp.StatusCode, filteredResponseHeaders(resp.Header), data)
			return
		}
		if finalizeErr := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "released", 0, fmt.Sprintf("upstream status %d", resp.StatusCode)); finalizeErr != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream rejection was received but local reservation could not be released")
			return
		}
		copyResponse(w, resp.StatusCode, filteredResponseHeaders(resp.Header), data)
		return
	}

	copyResponseHeaders(w, filteredResponseHeaders(resp.Header))
	w.WriteHeader(resp.StatusCode)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	_, copyErr := io.CopyBuffer(w, resp.Body, make([]byte, 32*1024))
	if copyErr != nil {
		if finalizeErr := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, "stream interrupted: "+copyErr.Error()); finalizeErr != nil {
			slog.Error("failed to persist interrupted streaming settlement", "request_id", requestID, "error", finalizeErr)
		}
		return
	}

	after, hasAfter := s.readMainBalance(r.Context(), ownerID)
	if !hasAfter {
		_ = s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, "owner balance could not be read after streaming relay")
		return
	}
	delta := before - after
	if delta < 0 {
		delta = 0
	}
	if delta == 0 {
		_ = s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", 0, "owner usage is not visible yet")
		_, _ = s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, after, "balance-pending:"+chargeID)
		return
	}
	if delta > s.cfg.MaxRequestCostCents {
		_ = s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "pending", delta, "actual owner charge exceeds local reservation")
		_, _ = s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, after, "balance-pending:"+chargeID)
		return
	}
	if finalizeErr := s.store.FinalizeSettlement(s.cfg.AgentID, chargeID, chargeID, "confirmed", delta, ""); finalizeErr != nil {
		slog.Error("failed to finalize streaming settlement", "request_id", requestID, "error", finalizeErr)
		return
	}
	_, _ = s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, after, "balance-after:"+chargeID)
}

type settlementReconcileResult struct {
	RequestID   string `json:"request_id"`
	Status      string `json:"status"`
	ActualCents int64  `json:"actual_cents"`
	UsageID     string `json:"usage_id,omitempty"`
	Message     string `json:"message,omitempty"`
}

func (s *Server) reconcileSettlements(ctx context.Context, requestID string) ([]settlementReconcileResult, error) {
	ownerID := strings.TrimSpace(s.cfg.OwnerMainUserID)
	if ownerID == "" {
		return nil, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_OWNER_MISSING", Message: "billing owner is not configured"}
	}
	// Reconciliation changes the same local reservation/allocated counters as
	// model relay and manual allocation. Serialize the whole observation and
	// finalization against the Owner so a concurrent request cannot spend a
	// balance snapshot while an older settlement is being released.
	ownerMu := s.userSettlementMutex(ownerID)
	ownerMu.Lock()
	defer ownerMu.Unlock()
	var records []SettlementRecord
	var err error
	if requestID != "" {
		record, lookupErr := s.store.Settlement(s.cfg.AgentID, requestID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		records = []SettlementRecord{record}
	} else {
		records, err = s.store.PendingSettlements(s.cfg.AgentID, s.cfg.SettlementReconcileBatch)
		if err != nil {
			return nil, err
		}
	}
	result := make([]settlementReconcileResult, 0, len(records))
	for _, record := range records {
		item := settlementReconcileResult{RequestID: record.RequestID, Status: record.Status, ActualCents: record.ActualCents, UsageID: record.UsageID}
		if record.Status == "released" || record.Status == "reversed" {
			item.Message = "settlement is already released"
			result = append(result, item)
			continue
		}
		usageItems, findErr := s.main.AdminFindUsage(ctx, record.RequestID)
		if findErr != nil {
			return nil, findErr
		}
		var usage *MainUsageResult
		for i := range usageItems {
			if usageItems[i].RequestID == record.RequestID {
				usage = &usageItems[i]
				break
			}
		}
		if usage == nil {
			item.Status = "pending"
			item.Message = "main-site usage is not visible yet; no local refund was made"
			result = append(result, item)
			continue
		}
		actual := usage.ActualCents
		if actual == 0 && usage.TotalCents > 0 {
			actual = usage.TotalCents
		}
		item.ActualCents = actual
		item.UsageID = usage.ID
		if actual > record.ReservedCents {
			if err := s.store.FinalizeSettlement(s.cfg.AgentID, record.RequestID, usage.ID, "pending", actual, "authoritative usage exceeds local reservation"); err != nil {
				return nil, err
			}
			item.Status = "pending"
			item.Message = "authoritative charge exceeds local reservation; manual review required"
			result = append(result, item)
			continue
		}
		if record.Status == "pending" {
			if err := s.store.FinalizeSettlement(s.cfg.AgentID, record.RequestID, usage.ID, "confirmed", actual, ""); err != nil && !errors.Is(err, errSettlementStateConflict) {
				return nil, err
			}
		}
		item.Status = "confirmed"
		item.Message = "settlement confirmed from main-site usage"
		result = append(result, item)
	}
	return result, nil
}

func (s *Server) writeSettlementReplay(w http.ResponseWriter, requestID string, settlement SettlementRecord) {
	if settlement.Status == "pending" {
		s.writeError(w, http.StatusConflict, requestID, "SETTLEMENT_PENDING", "this request is already being reconciled; retry with a new request id only after it is resolved")
		return
	}
	s.writeError(w, http.StatusConflict, requestID, "IDEMPOTENCY_REPLAY", "this request id has already been settled and will not be forwarded again")
}

type modelPrincipal struct {
	ProxyMainUserID string
	User            AgentUserView
}

// requireModelPrincipal accepts either the browser session or an AgentAPI
// gateway key. The latter is intentionally resolved only against the local
// agent_api_keys table, so a Sub2API JWT, administrator key, or arbitrary
// bearer token cannot be used as a model credential here.
func (s *Server) requireModelPrincipal(w http.ResponseWriter, r *http.Request, requestID string) (modelPrincipal, bool) {
	if session, user, ok := s.loadSession(r); ok {
		// Cookie-authenticated model requests are state-changing from the
		// browser's perspective. Require the page origin to match the AgentAPI
		// host; programmatic clients should use a local sk-agent-* key instead.
		if r.Method != http.MethodGet && !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin model requests are not allowed")
			return modelPrincipal{}, false
		}
		return modelPrincipal{ProxyMainUserID: session.MainUserID, User: user}, true
	}

	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" {
		s.writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "AgentAPI session or API key is required")
		return modelPrincipal{}, false
	}
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "Bearer") || strings.TrimSpace(parts[1]) == "" {
		s.writeError(w, http.StatusUnauthorized, requestID, "INVALID_AGENT_API_KEY", "a valid AgentAPI bearer key is required")
		return modelPrincipal{}, false
	}
	mainUserID, err := s.store.ResolveAPIKey(s.cfg.AgentID, strings.TrimSpace(parts[1]))
	if err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusUnauthorized, requestID, "INVALID_AGENT_API_KEY", "AgentAPI API key is invalid or revoked")
			return modelPrincipal{}, false
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to resolve AgentAPI API key")
		return modelPrincipal{}, false
	}
	user, err := s.store.User(s.cfg.AgentID, mainUserID)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, requestID, "AGENT_USER_NOT_FOUND", "AgentAPI user mapping is unavailable")
		return modelPrincipal{}, false
	}
	if user.Status != "active" {
		s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_DISABLED", "agent user is disabled")
		return modelPrincipal{}, false
	}
	return modelPrincipal{ProxyMainUserID: mainUserID, User: user}, true
}

func (s *Server) readMainBalance(ctx context.Context, mainUserID string) (int64, bool) {
	if s.cfg.AdminKey == "" {
		return 0, false
	}
	user, err := s.main.AdminGetUser(ctx, mainUserID)
	if err != nil {
		return 0, false
	}
	return user.Balance, true
}

func (s *Server) mainBalance(ctx context.Context, mainUserID string) (int64, bool) {
	mu := s.userSettlementMutex(mainUserID)
	mu.Lock()
	defer mu.Unlock()
	return s.readMainBalance(ctx, mainUserID)
}

func (s *Server) syncOwnerBalance(ctx context.Context, requestID string) (AgentView, error) {
	ownerID := strings.TrimSpace(s.cfg.OwnerMainUserID)
	if ownerID == "" {
		return AgentView{}, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_OWNER_MISSING", Message: "billing owner is not configured"}
	}
	if strings.TrimSpace(s.cfg.AdminKey) == "" {
		return AgentView{}, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_ADMIN_KEY_MISSING", Message: "main administrator credential is not configured"}
	}
	mu := s.userSettlementMutex(ownerID)
	mu.Lock()
	defer mu.Unlock()
	user, err := s.main.AdminGetUser(ctx, ownerID)
	if err != nil {
		return AgentView{}, err
	}
	return s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, user.Balance, requestID)
}

// syncAndAllocateRechargeOrder holds the same per-owner mutex used by model
// relay settlement for the entire sync+allocation sequence. Without this
// critical section a model request could spend the owner balance after the
// snapshot but before a paid recharge consumes the local available amount.
func (s *Server) syncAndAllocateRechargeOrder(ctx context.Context, requestID, orderNo string) (RechargeOrder, error) {
	ownerID := strings.TrimSpace(s.cfg.OwnerMainUserID)
	if ownerID == "" {
		return RechargeOrder{}, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_OWNER_MISSING", Message: "billing owner is not configured"}
	}
	if strings.TrimSpace(s.cfg.AdminKey) == "" {
		return RechargeOrder{}, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_ADMIN_KEY_MISSING", Message: "main admin key is not configured"}
	}
	mu := s.userSettlementMutex(ownerID)
	mu.Lock()
	defer mu.Unlock()
	user, err := s.main.AdminGetUser(ctx, ownerID)
	if err != nil {
		return RechargeOrder{}, err
	}
	if _, err := s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, user.Balance, requestID); err != nil {
		return RechargeOrder{}, err
	}
	return s.store.AllocateRechargeOrder(s.cfg.AgentID, orderNo)
}

// syncAndAllocateUser keeps the authoritative Owner balance snapshot and the
// local user allocation in the same per-owner critical section used by model
// relay settlement. Otherwise a model request could spend the Owner balance
// after the snapshot but before the allocation, creating local credit that no
// longer exists upstream.
func (s *Server) syncAndAllocateUser(ctx context.Context, requestID, mainUserID string, cents int64, allocationRequestID, orderID, note string) (AgentUserView, error) {
	ownerID := strings.TrimSpace(s.cfg.OwnerMainUserID)
	if ownerID == "" {
		return AgentUserView{}, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_OWNER_MISSING", Message: "billing owner is not configured"}
	}
	if strings.TrimSpace(s.cfg.AdminKey) == "" {
		return AgentUserView{}, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_ADMIN_KEY_MISSING", Message: "main administrator credential is not configured"}
	}
	mu := s.userSettlementMutex(ownerID)
	mu.Lock()
	defer mu.Unlock()
	user, err := s.store.User(s.cfg.AgentID, mainUserID)
	if err != nil {
		return AgentUserView{}, err
	}
	owner, err := s.main.AdminGetUser(ctx, ownerID)
	if err != nil {
		return AgentUserView{}, err
	}
	if _, err := s.store.SyncOwnerBalance(s.cfg.AgentID, ownerID, owner.Balance, requestID); err != nil {
		return AgentUserView{}, err
	}
	if err := s.store.Allocate(s.cfg.AgentID, mainUserID, cents, allocationRequestID, orderID, note); err != nil {
		return AgentUserView{}, err
	}
	return s.store.User(s.cfg.AgentID, user.MainUserID)
}

func (s *Server) userSettlementMutex(id string) *sync.Mutex {
	s.settleMu.Lock()
	defer s.settleMu.Unlock()
	if mu := s.settleByUser[id]; mu != nil {
		return mu
	}
	mu := &sync.Mutex{}
	s.settleByUser[id] = mu
	return mu
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request, requestID string) (Session, AgentUserView, bool) {
	session, user, ok := s.loadSession(r)
	if !ok {
		s.writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "AgentAPI session is required")
		return Session{}, AgentUserView{}, false
	}
	return session, user, true
}

func (s *Server) loadSession(r *http.Request) (Session, AgentUserView, bool) {
	cookie, err := r.Cookie(s.cfg.CookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return Session{}, AgentUserView{}, false
	}
	session, err := s.store.LoadSession(cookie.Value)
	if err != nil {
		return Session{}, AgentUserView{}, false
	}
	user, err := s.store.User(s.cfg.AgentID, session.MainUserID)
	if err != nil || user.Status != "active" {
		return Session{}, AgentUserView{}, false
	}
	return session, user, true
}

func (s *Server) isAgentAdmin(session Session) bool {
	// Agent administration is an explicit per-instance ownership grant. A
	// user's global role on Sub2API must not silently grant control of every
	// AgentAPI wallet reachable through this instance.
	return s.cfg.OwnerMainUserID != "" && session.MainUserID == s.cfg.OwnerMainUserID
}

func redactAgentFinancials(agent AgentView) AgentView {
	agent.OwnerMainUserID = ""
	agent.MainBalance = 0
	agent.BalanceChecked = ""
	agent.BillingStatus = "redacted"
	agent.WalletAvailable = 0
	agent.WalletAllocated = 0
	return agent
}

func (s *Server) browserAuthResponse(raw []byte, mainUserID string) map[string]any {
	return map[string]any{
		"access_token":  "",
		"refresh_token": "",
		"expires_in":    int(sessionTTL.Seconds()),
		"token_type":    "Cookie",
		"user":          s.browserUser(raw, mainUserID),
	}
}

func (s *Server) browserUser(raw []byte, mainUserID string) map[string]any {
	result := make(map[string]any)
	if json.Unmarshal(raw, &result) != nil {
		result = map[string]any{}
	}
	delete(result, "access_token")
	delete(result, "refresh_token")
	delete(result, "token")
	if numericID, err := strconv.ParseInt(mainUserID, 10, 64); err == nil && numericID > 0 {
		result["id"] = numericID
	} else {
		result["id"] = mainUserID
	}
	// AgentAPI's copied admin UI must not become a main-site admin console. The
	// separate agent_admin flag is consumed by the AgentAPI view only.
	result["role"] = "user"
	var sessionUser Session
	_ = sessionUser
	result["agent_admin"] = s.cfg.OwnerMainUserID == mainUserID
	result["agent_id"] = s.cfg.AgentID
	return result
}

func (s *Server) setSessionCookie(w http.ResponseWriter, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: s.cfg.CookieName, Value: value, Path: "/", HttpOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode,
		Expires: expiresAt, MaxAge: int(sessionTTL.Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: s.cfg.CookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func (s *Server) writeData(w http.ResponseWriter, status int, requestID string, data any) {
	s.writeJSON(w, status, apiResponse{Code: 0, Message: "success", RequestID: requestID, Data: data})
}

func (s *Server) recordAudit(actorType, actorID, operation, targetType, targetID, requestID, result, reason string) {
	if err := s.store.RecordAuditEvent(AuditEvent{
		ActorType: actorType, ActorID: actorID, AgentID: s.cfg.AgentID,
		Operation: operation, TargetType: targetType, TargetID: targetID,
		RequestID: requestID, Result: result, Reason: reason,
	}); err != nil {
		slog.Error("failed to persist audit event", "agent_id", s.cfg.AgentID, "operation", operation, "request_id", requestID, "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, requestID, reason, message string) {
	s.writeJSON(w, status, apiResponse{Code: status, Message: message, Reason: reason, RequestID: requestID})
}

func (s *Server) writeMainError(w http.ResponseWriter, requestID string, err error) {
	var apiErr *MainAPIError
	if errors.As(err, &apiErr) {
		status := apiErr.Status
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		s.writeError(w, status, requestID, apiErr.Code, apiErr.Message)
		return
	}
	s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_UNAVAILABLE", "main site is temporarily unavailable")
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload apiResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) serveWeb(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.webDir) == "" {
		s.writeError(w, http.StatusNotFound, requestID(r), "WEB_NOT_BUILT", "frontend is not built")
		return
	}
	clean := path.Clean("/" + r.URL.Path)
	if strings.Contains(clean, "..") {
		s.writeError(w, http.StatusBadRequest, requestID(r), "INVALID_PATH", "invalid path")
		return
	}
	file := filepath.Join(s.webDir, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	if info, err := os.Stat(file); err == nil && !info.IsDir() {
		http.ServeFile(w, r, file)
		return
	}
	index := filepath.Join(s.webDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		s.writeError(w, http.StatusNotFound, requestID(r), "WEB_NOT_BUILT", "frontend is not built")
		return
	}
	http.ServeFile(w, r, index)
}

func copyResponse(w http.ResponseWriter, status int, headers http.Header, body []byte) {
	copyResponseHeaders(w, headers)
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func copyResponseHeaders(w http.ResponseWriter, headers http.Header) {
	for key, values := range headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
}

func isAllowedUserAPIPath(rawPath, method string) bool {
	// AgentAPI deliberately has no generic main-site API proxy. The explicit
	// routes above are the complete public contract; forwarding an unknown
	// /api/v1 path would let a browser bypass local wallet checks or reach
	// account, payment, key, subscription, and administrator resources.
	_ = rawPath
	_ = method
	return false
}

func isAllowedModelPath(rawPath, method string) bool {
	if method != http.MethodGet && method != http.MethodPost {
		return false
	}
	switch rawPath {
	case "/v1/models", "/v1/chat/completions", "/v1/responses", "/v1/images/generations", "/v1/images/edits", "/v1/videos":
		return true
	default:
		videoID := strings.TrimPrefix(rawPath, "/v1/videos/")
		return method == http.MethodGet && strings.HasPrefix(rawPath, "/v1/videos/") && videoID != "" && videoID != "." && videoID != ".." && !strings.Contains(videoID, "/")
	}
}

func sameOrigin(r *http.Request) bool {
	return sameOriginOrMachine(r)
}

func sameOriginOrMachine(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	// Do not trust a client-supplied forwarded-host header here. The reverse
	// proxy should preserve the public Host header (as shown in the deployment
	// document); accepting arbitrary X-Forwarded-Host values would let a direct
	// caller forge a same-origin CORS response.
	requestHost := strings.TrimSpace(r.Host)
	if strings.EqualFold(parsed.Host, requestHost) {
		return true
	}
	// Vite's local proxy and other local development proxies use different
	// ports for the page and API. Permit only loopback hostnames in that case.
	originHost := parsed.Hostname()
	requestName := requestHost
	if host, _, splitErr := net.SplitHostPort(requestHost); splitErr == nil {
		requestName = host
	} else if strings.Contains(requestHost, ":") && !strings.HasPrefix(requestHost, "[") {
		requestName = strings.SplitN(requestHost, ":", 2)[0]
	}
	return isLoopbackHost(originHost) && isLoopbackHost(requestName)
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

var publicModelNames = map[string]struct{}{
	"gpt-5.5": {}, "gpt-5.4-mini": {}, "gpt-5.6-" + "luna": {}, "deepseek-chat": {},
	"gpt-image-2": {}, "gpt-image-1.5": {}, "gpt-image-1": {},
	"gemini-3.1-flash-image": {}, "grok-imagine-image-1.5": {}, "grok-imagine-image": {},
	"grok-imagine-image-2.0": {}, "grok-imagine-image-quality": {}, "grok-imagine": {},
	"grok-imagine-video-1.5": {}, "seedance-2.0": {}, "seedance-2.0-fast": {},
	"kling-v3": {}, "kling-v3-omni": {}, "kling-v2-6": {}, "kling-v2-5-turbo": {},
	"kling-v1-6": {}, "kling-v1-5": {}, "kling-v1": {},
}

var publicModelCatalog = []string{
	"gpt-5.5", "gpt-5.4-mini", "gpt-5.6-" + "luna", "deepseek-chat",
	"gpt-image-2", "gpt-image-1.5", "gpt-image-1",
	"gemini-3.1-flash-image", "grok-imagine-image-1.5", "grok-imagine-image",
	"grok-imagine-image-2.0", "grok-imagine-image-quality", "grok-imagine",
	"grok-imagine-video-1.5", "seedance-2.0", "seedance-2.0-fast",
	"kling-v3", "kling-v3-omni", "kling-v2-6", "kling-v2-5-turbo",
	"kling-v1-6", "kling-v1-5", "kling-v1",
}

func (s *Server) writePublicModels(w http.ResponseWriter) {
	models := make([]map[string]any, 0, len(publicModelCatalog))
	for _, name := range publicModelCatalog {
		models = append(models, map[string]any{
			"id": name, "object": "model", "created": 0, "owned_by": "agentapi",
		})
	}
	writeModelJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
}

func writeModelJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func validatePublicModel(contentType string, body []byte) error {
	if strings.Contains(strings.ToLower(contentType), "multipart/") {
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
			// Leave malformed multipart bodies to the upstream protocol parser.
			return nil
		}
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, nextErr := reader.NextPart()
			if errors.Is(nextErr, io.EOF) {
				return nil
			}
			if nextErr != nil {
				// Preserve the upstream's normal malformed-multipart error.
				return nil
			}
			if part.FormName() != "model" {
				continue
			}
			modelBytes, readErr := io.ReadAll(io.LimitReader(part, 256))
			if readErr != nil {
				return nil
			}
			model := strings.TrimSpace(string(modelBytes))
			if model == "" {
				return nil
			}
			if _, ok := publicModelNames[model]; !ok {
				return fmt.Errorf("model %q is not in the AgentAPI public model catalog", model)
			}
			return nil
		}
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		// Let the upstream protocol parser return its normal invalid-JSON error.
		return nil
	}
	model := strings.TrimSpace(payload.Model)
	if model == "" {
		return nil
	}
	if _, ok := publicModelNames[model]; !ok {
		return fmt.Errorf("model %q is not in the AgentAPI public model catalog", model)
	}
	return nil
}

func modelRequestWantsStream(accept, contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(accept), "text/event-stream") {
		return true
	}
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		var payload struct {
			Stream bool `json:"stream"`
		}
		if json.Unmarshal(body, &payload) == nil && payload.Stream {
			return true
		}
	}
	return false
}

func decodeJSON(r *http.Request, target any, limit int64) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("request body is too large")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return fmt.Errorf("request body is required")
	}
	return json.Unmarshal(body, target)
}

// decodeOptionalJSON accepts an empty body for endpoints where the body is a
// filter rather than the operation itself. Reading the stream instead of
// relying on Content-Length also handles chunked requests correctly.
func decodeOptionalJSON(r *http.Request, target any, limit int64) (bool, error) {
	if r.Body == nil {
		return false, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return false, err
	}
	if int64(len(body)) > limit {
		return false, fmt.Errorf("request body is too large")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return true, err
	}
	return true, nil
}

func requestID(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get("X-Request-ID")); id != "" && len(id) <= 128 {
		return id
	}
	return randomID("req_")
}

func requestIDFrom(r *http.Request, payload map[string]any) string {
	if id := strings.TrimSpace(r.Header.Get("Idempotency-Key")); id != "" && len(id) <= 128 {
		return id
	}
	if payload != nil {
		for _, key := range []string{"request_id", "order_id", "source_order_no"} {
			if id := strings.TrimSpace(stringValue(payload[key])); id != "" && len(id) <= 128 {
				return id
			}
		}
	}
	return requestID(r)
}

func randomID(prefix string) string {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err != nil {
		return prefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return prefix + hex.EncodeToString(data)
}

func amountFromPayload(payload map[string]any) (int64, error) {
	if raw, ok := payload["amount_cents"]; ok {
		switch value := raw.(type) {
		case float64:
			if value != float64(int64(value)) {
				return 0, fmt.Errorf("amount_cents must be an integer")
			}
			return int64(value), nil
		case string:
			return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		}
	}
	if raw, ok := payload["amount"]; ok {
		switch value := raw.(type) {
		case float64:
			return parseCents(strconv.FormatFloat(value, 'f', -1, 64))
		case string:
			return parseCents(value)
		}
	}
	return 0, fmt.Errorf("amount is required")
}
