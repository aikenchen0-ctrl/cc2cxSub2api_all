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
}

type apiResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Reason    string `json:"reason,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Data      any    `json:"data,omitempty"`
}

func main() {
	preflightMode := len(os.Args) == 2 && os.Args[1] == "--preflight"
	backupMode := len(os.Args) == 3 && os.Args[1] == "--backup"
	if len(os.Args) > 1 && !preflightMode && !backupMode {
		slog.Error("usage: agentapi [--preflight | --backup <destination.db>]")
		os.Exit(2)
	}
	cfg, err := LoadConfig()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}
	if preflightMode {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		report := runPreflight(ctx, cfg)
		if err := json.NewEncoder(os.Stdout).Encode(report); err != nil || !report.OK {
			os.Exit(1)
		}
		return
	}
	if backupMode {
		if err := backupSQLiteDatabase(cfg.DatabasePath, os.Args[2]); err != nil {
			slog.Error("database backup failed", "error", err)
			os.Exit(1)
		}
		slog.Info("database backup completed")
		return
	}
	store, err := OpenStore(cfg.DatabasePath, cfg.SessionSecret)
	if err != nil {
		slog.Error("open store failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if strings.TrimSpace(cfg.AgentID) != "" {
		if err := store.UpsertAgent(cfg); err != nil {
			slog.Error("initialize compatibility tenant failed", "error", err)
			os.Exit(1)
		}
	}

	server := &Server{
		cfg: cfg, store: store, main: NewMainClient(cfg), webDir: cfg.WebDir,
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
	slog.Info("agentapi shared runtime started", "addr", cfg.Addr)
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
		s.handleHealth(w, r, requestID)
	case r.URL.Path == "/readyz":
		s.handleReady(w, r, requestID)
	case r.URL.Path == "/api/auth/sso/callback" || r.URL.Path == "/api/v1/auth/sso/callback":
		s.handleSSOCallback(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/auth/"):
		s.handleAuth(w, r, requestID)
	case r.URL.Path == "/api/v1/settings/public":
		s.handlePublicSettings(w, r, requestID)
	case r.URL.Path == "/api/v1/public/models":
		s.handlePublicModels(w, r, requestID)
	case r.URL.Path == "/api/v1/payments/webhook":
		// Payment providers call this endpoint without an AgentAPI cookie. Keep
		// it before the generic /api/v1 branch and authenticate it with the
		// provider-neutral HMAC contract in handlePaymentWebhook.
		s.handlePaymentWebhook(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/context":
		s.handleAgentContext(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/profile":
		s.handleAgentProfile(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/profile/avatar":
		s.handleAgentProfileAvatar(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/profile/bindings" || strings.HasPrefix(r.URL.Path, "/api/v1/agent/profile/bindings/"):
		s.handleAgentProfileBindings(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/passkeys" || strings.HasPrefix(r.URL.Path, "/api/v1/agent/passkeys/"):
		s.handleAgentPasskeys(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/password":
		s.handleAgentPassword(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/agent/totp/"):
		s.handleAgentTOTP(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/balance-notify" || strings.HasPrefix(r.URL.Path, "/api/v1/agent/balance-notify/"):
		s.handleAgentBalanceNotify(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/wallet":
		s.handleAgentWallet(w, r, requestID)
	case isForbiddenAgentManagementPath(r.URL.Path):
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "endpoint not found")
	case r.URL.Path == "/api/v1/agent/usage/insights":
		s.handleUsageInsights(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/usage":
		s.handleAgentUsage(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/affiliate" || strings.HasPrefix(r.URL.Path, "/api/v1/agent/affiliate/"):
		s.handleAgentAffiliate(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/orders" || strings.HasPrefix(r.URL.Path, "/api/v1/agent/orders/"):
		s.handleAgentOrders(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/agent/payment/"):
		s.handleAgentPayment(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/recharge/orders":
		s.handleAgentRecharge(w, r, requestID)
	case r.URL.Path == "/api/v1/agent/announcements" || strings.HasPrefix(r.URL.Path, "/api/v1/agent/announcements/"):
		s.handleAgentAnnouncements(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/agent/admin/"):
		s.handleAgentAdmin(w, r, requestID)
	case r.URL.Path == "/api/v1/api-keys" || strings.HasPrefix(r.URL.Path, "/api/v1/api-keys/"):
		s.handleAPIKeys(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/"):
		// Only explicit AgentAPI routes are exposed; never proxy arbitrary
		// main-site account or administrator endpoints with a user session.
		s.writeError(w, http.StatusForbidden, requestID, "AGENT_ROUTE_FORBIDDEN", "this main-site route is not exposed by AgentAPI")
	case strings.HasPrefix(r.URL.Path, "/v1/"):
		s.handleModelRelay(w, r, requestID)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		s.serveWeb(w, r)
	default:
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "not found")
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if s.store == nil || s.store.db == nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "STORE_UNAVAILABLE", "local persistence is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.db.PingContext(ctx); err != nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "STORE_UNAVAILABLE", "local persistence is unavailable")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"status": "ok"})
}

// isForbiddenAgentManagementPath is the permanent product boundary for the
// shared AgentAPI site. These modules belong to Sub2API or to internal
// operations and must never become tenant-administrator features.
func isForbiddenAgentManagementPath(path string) bool {
	if strings.HasPrefix(path, "/api/v1/agent/admin/users/") {
		return true
	}
	for _, prefix := range []string{
		"/api/v1/agent/users",
		"/api/v1/agent/admin/groups",
		"/api/v1/agent/admin/accounts",
		"/api/v1/agent/admin/agents",
		"/api/v1/agent/admin/proxies",
		"/api/v1/agent/admin/agent-provisioning",
		"/api/v1/agent/admin/ops",
		"/api/v1/agent/admin/promo-codes",
		"/api/v1/agent/admin/redeem",
		"/api/v1/agent/admin/redeem-codes",
		"/api/v1/agent/admin/plugins",
		"/api/v1/agent/admin/audit",
		"/api/v1/agent/admin/audit-logs",
		"/api/v1/agent/admin/audit-events",
		"/api/v1/agent/admin/risk-control",
		"/api/v1/agent/admin/prompt-audit",
		"/api/v1/agent/admin/security-audit",
		"/api/v1/agent/admin/upstream-audit",
		"/api/v1/agent/admin/upstream-verification",
		"/api/v1/agent/admin/channels",
		"/api/v1/agent/admin/subscriptions",
		"/api/v1/agent/admin/payment/plans",
		"/api/v1/agent/admin/satellite-billing",
		"/api/v1/agent/admin/content",
		"/api/v1/agent/admin/content-pages",
		"/api/v1/agent/content-pages",
		"/api/v1/agent/content",
		"/api/v1/agent/admin/backup",
		"/api/v1/agent/admin/backups",
		"/api/v1/agent/admin/model-policy",
		"/api/v1/agent/tasks",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func (s *Server) allowedHost(rawHost string) bool {
	host, ok := normalizeTenantHost(rawHost)
	if !ok {
		return false
	}
	if _, err := s.store.AgentByDomain(host); err == nil {
		return true
	}
	for _, sharedHost := range s.cfg.SharedHosts {
		if host == sharedHost {
			return true
		}
	}
	expected, configured := normalizeTenantHost(s.cfg.AgentDomain)
	if configured {
		return host == expected
	}
	// Preserve the deliberately enabled single-tenant migration mode. Shared
	// production has no process-wide AGENT_ID and therefore never reaches this
	// compatibility fallback.
	return strings.TrimSpace(s.cfg.AgentID) != ""
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
	if strings.TrimSpace(s.cfg.AppCredential) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(parent, s.cfg.MainRequestTimeout)
	defer cancel()
	agents, err := s.store.ActiveAgents()
	if err != nil {
		slog.Warn("background tenant lookup failed", "error", err)
		return
	}
	for _, agent := range agents {
		items, err := s.reconcileTenantSettlements(ctx, agent.ID, "")
		if err != nil {
			slog.Warn("background settlement reconciliation failed", "agent_id", agent.ID, "error", err)
		} else if len(items) > 0 {
			slog.Info("background settlement reconciliation completed", "agent_id", agent.ID, "count", len(items))
		}
		s.reconcilePaidRechargeOrdersForTenant(ctx, agent)
		s.reconcileStaleVideoTasksForTenant(ctx, agent.ID)
		s.reconcileStaleImageTasksForTenant(ctx, agent.ID)
	}
}

func (s *Server) reconcileStaleVideoTasksForTenant(ctx context.Context, agentID string) {
	if s.cfg.VideoTaskReconcileAge <= 0 || strings.TrimSpace(s.cfg.AppCredential) == "" || s.main == nil {
		return
	}
	agentID = strings.TrimSpace(agentID)
	tasks, err := s.store.PendingVideoTasks(agentID, s.cfg.SettlementReconcileBatch)
	if err != nil {
		slog.Warn("video task reconciliation lookup failed", "agent_id", agentID, "error", err)
		return
	}
	cutoff := time.Now().UTC().Add(-s.cfg.VideoTaskReconcileAge)
	for _, task := range tasks {
		createdAt, parseErr := time.Parse(time.RFC3339, task.CreatedAt)
		if parseErr != nil || createdAt.After(cutoff) {
			continue
		}
		billingUserID := strings.TrimSpace(task.MainUserID)
		if settlement, settlementErr := s.store.Settlement(agentID, task.RequestID); settlementErr == nil && strings.TrimSpace(settlement.BillingMainUserID) != "" {
			billingUserID = strings.TrimSpace(settlement.BillingMainUserID)
		}
		leaseCtx, releaseLease, leaseErr := s.acquireUserOperationLease(ctx, billingUserID)
		if leaseErr != nil {
			slog.Warn("video task reconciliation lease unavailable", "agent_id", agentID, "main_user_id", billingUserID, "error", leaseErr)
			continue
		}
		status, _, body, relayErr := s.main.RelayModelMethod(leaseCtx, http.MethodGet, "/v1/videos/"+url.PathEscape(task.TaskID), nil, nil, s.relayBillingIdentity(task.MainUserID), task.RequestID)
		shouldReconcile := false
		if relayErr == nil && status >= 200 && status < 300 {
			_, upstreamStatus := videoTaskIdentity(body)
			if upstreamStatus != "" {
				_ = s.store.UpdateVideoTaskStatus(agentID, task.TaskID, upstreamStatus)
				if videoTaskFailed(upstreamStatus) {
					settlement, settlementErr := s.store.Settlement(agentID, task.RequestID)
					if settlementErr == nil && settlement.Status == "pending" {
						usage, available, usageErr := s.mainUsageForRequest(leaseCtx, billingUserID, task.RequestID)
						switch {
						case !available:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "failed video task requires authoritative usage lookup; legacy usage API is unavailable")
						case usageErr != nil:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "failed video task usage lookup failed: "+usageErr.Error())
						case usage == nil:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "video task failed; awaiting authoritative main-site usage: "+upstreamStatus)
						case settlement.HasLocalReservation && usage.ActualCents > settlement.ReservedCents:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, usage.ID, "pending", usage.ActualCents, "authoritative video usage exceeds local reservation", usage.Snapshot)
						default:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, usage.ID, "confirmed", usage.ActualCents, "", usage.Snapshot)
						}
					}
				} else {
					shouldReconcile = true
				}
			}
		}
		releaseLease()
		if shouldReconcile {
			_, _ = s.reconcileTenantSettlements(ctx, agentID, task.RequestID)
		}
		if relayErr != nil {
			slog.Warn("stale video task probe failed", "agent_id", agentID, "task_id", task.TaskID, "error", relayErr)
		}
	}
}

func (s *Server) reconcileStaleImageTasksForTenant(ctx context.Context, agentID string) {
	if s.cfg.ImageTaskReconcileAge <= 0 || strings.TrimSpace(s.cfg.AppCredential) == "" || s.main == nil {
		return
	}
	agentID = strings.TrimSpace(agentID)
	tasks, err := s.store.PendingImageTasks(agentID, s.cfg.SettlementReconcileBatch)
	if err != nil {
		slog.Warn("image task reconciliation lookup failed", "agent_id", agentID, "error", err)
		return
	}
	cutoff := time.Now().UTC().Add(-s.cfg.ImageTaskReconcileAge)
	for _, task := range tasks {
		createdAt, parseErr := time.Parse(time.RFC3339, task.CreatedAt)
		if parseErr != nil || createdAt.After(cutoff) {
			continue
		}
		billingUserID := strings.TrimSpace(task.MainUserID)
		if settlement, settlementErr := s.store.Settlement(agentID, task.RequestID); settlementErr == nil && strings.TrimSpace(settlement.BillingMainUserID) != "" {
			billingUserID = strings.TrimSpace(settlement.BillingMainUserID)
		}
		leaseCtx, releaseLease, leaseErr := s.acquireUserOperationLease(ctx, billingUserID)
		if leaseErr != nil {
			slog.Warn("image task reconciliation lease unavailable", "agent_id", agentID, "main_user_id", billingUserID, "error", leaseErr)
			continue
		}
		status, _, body, relayErr := s.main.RelayModelMethod(leaseCtx, http.MethodGet, "/v1/images/tasks/"+url.PathEscape(task.TaskID), nil, nil, s.relayBillingIdentity(task.MainUserID), task.RequestID)
		shouldReconcile := false
		if relayErr == nil && status >= 200 && status < 300 {
			_, imageStatus := imageTaskIdentity(body)
			if imageStatus != "" {
				_ = s.store.UpdateImageTaskStatus(agentID, task.TaskID, imageStatus)
				if imageTaskFailed(imageStatus) {
					settlement, settlementErr := s.store.Settlement(agentID, task.RequestID)
					if settlementErr == nil && settlement.Status == "pending" {
						usage, available, usageErr := s.mainUsageForRequest(leaseCtx, billingUserID, task.RequestID)
						switch {
						case !available:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "failed image task requires authoritative usage lookup; legacy usage API is unavailable")
						case usageErr != nil:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "failed image task usage lookup failed: "+usageErr.Error())
						case usage == nil:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "image task failed; awaiting authoritative main-site usage: "+imageStatus)
						case settlement.HasLocalReservation && usage.ActualCents > settlement.ReservedCents:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, usage.ID, "pending", usage.ActualCents, "authoritative image usage exceeds local reservation", usage.Snapshot)
						default:
							_ = s.store.FinalizeSettlement(agentID, task.RequestID, usage.ID, "confirmed", usage.ActualCents, "", usage.Snapshot)
						}
					}
				} else {
					shouldReconcile = true
				}
			}
		}
		releaseLease()
		if shouldReconcile {
			_, _ = s.reconcileTenantSettlements(ctx, agentID, task.RequestID)
		}
		if relayErr != nil {
			slog.Warn("stale image task probe failed", "agent_id", agentID, "task_id", task.TaskID, "error", relayErr)
		}
	}
}

func (s *Server) reconcilePaidRechargeOrdersForTenant(ctx context.Context, agent AgentView) {
	payment := s.paymentConfigFor(agent.ID)
	if s.tenantBillingMode(agent) != "owner_upstream" || !payment.Enabled || strings.TrimSpace(s.tenantOwnerMainUserID(agent)) == "" || strings.TrimSpace(s.cfg.AppCredential) == "" {
		return
	}
	orders, err := s.store.PaidPendingRechargeOrders(agent.ID, s.cfg.SettlementReconcileBatch)
	if err != nil {
		slog.Warn("background recharge reconciliation lookup failed", "agent_id", agent.ID, "error", err)
		return
	}
	for _, order := range orders {
		if _, err := s.syncAndAllocateRechargeOrderForTenant(ctx, agent.ID, "payment-reconcile:"+order.OrderNo, order.OrderNo); err != nil {
			if errors.Is(err, errInsufficientBalance) {
				// Keep the order pending until the owner tops up. Do not turn a
				// verified payment into a failure merely because credit is delayed.
				continue
			}
			slog.Warn("background recharge allocation failed", "agent_id", agent.ID, "order_no", order.OrderNo, "error", err)
			continue
		}
		slog.Info("background recharge allocation completed", "agent_id", agent.ID, "order_no", order.OrderNo)
	}
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if strings.TrimSpace(s.cfg.AppCredential) == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "MAIN_APP_CREDENTIAL_MISSING", "model relay credential is not configured")
		return
	}
	if len(s.cfg.SSOSecret) < 32 {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "SSO_SECRET_WEAK", "SSO secret must contain at least 32 characters")
		return
	}
	if s.cfg.SessionSecretWeak {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "SESSION_SECRET_WEAK", "session secret must contain at least 32 characters")
		return
	}
	agents, err := s.store.ActiveAgents()
	if err != nil || len(agents) == 0 {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "AGENT_NOT_READY", "no active tenant configuration is ready")
		return
	}
	for _, agent := range agents {
		switch s.tenantBillingMode(agent) {
		case "user_upstream":
			// The signed-in user's own Sub2API balance is authoritative; no
			// tenant owner wallet is required for this mode.
		case "owner_upstream":
			if strings.TrimSpace(agent.OwnerMainUserID) == "" {
				s.writeError(w, http.StatusServiceUnavailable, requestID, "MAIN_OWNER_MISSING", "an active owner-billed tenant has no billing owner")
				return
			}
		default:
			s.writeError(w, http.StatusServiceUnavailable, requestID, "BILLING_MODE_INVALID", "an active tenant has an invalid billing mode")
			return
		}
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"status": "ready", "active_tenants": len(agents)})
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
	case "/api/v1/auth/send-verify-code":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authSendVerifyCode(w, r, requestID)
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
	case "/api/v1/auth/passkey/config":
		s.handleAgentPasskeyConfig(w, r, requestID)
	case "/api/v1/auth/passkey/login/begin":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authPasskeyLoginBegin(w, r, requestID)
	case "/api/v1/auth/passkey/login/finish":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authPasskeyLoginFinish(w, r, requestID)
	case "/api/v1/auth/forgot-password":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authForgotPassword(w, r, requestID)
	case "/api/v1/auth/reset-password":
		if r.Method != http.MethodPost || !sameOrigin(r) {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authResetPassword(w, r, requestID)
	case "/api/v1/auth/password-recovery/config":
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authPasswordRecoveryConfig(w, r, requestID)
	case "/api/v1/auth/main-site/login":
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.authMainSiteLogin(w, r, requestID)
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

// authMainSiteLogin starts the existing Sub2API satellite SSO flow. The
// browser receives only a public navigation URL; application credentials,
// runtime-control credentials and the SSO secret remain server-side.
func (s *Server) authMainSiteLogin(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	base, ok := publicMainOrigin(s.cfg.PublicMainURL)
	if !ok {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "MAIN_SITE_LOGIN_UNAVAILABLE", "main-site login is not configured")
		return
	}
	base.Path = "/api/v1/auth/integrations/" + url.PathEscape(s.cfg.SatelliteSlug) + "/start"
	query := base.Query()
	query.Set("next", safeSSONext(r.URL.Query().Get("next")))
	base.RawQuery = query.Encode()
	http.Redirect(w, r, base.String(), http.StatusFound)
}

func publicMainOrigin(raw string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, false
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return nil, false
	}
	parsed.Path = ""
	return parsed, true
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
	rawTicket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	ticket, err := verifySSOTicket(rawTicket, secret, s.cfg.SSOAudience, time.Now().UTC())
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, errIdempotencyConflict) {
			status = http.StatusConflict
		}
		s.writeError(w, status, requestID, "SSO_TICKET_INVALID", err.Error())
		return
	}
	agent, role, resolveErr := s.resolveSSOTicketAgent(r, ticket)
	if resolveErr != nil {
		status := http.StatusMisdirectedRequest
		reason := "TENANT_NOT_FOUND"
		if errors.Is(resolveErr, errSSOTenantForbidden) {
			status = http.StatusForbidden
			reason = "SSO_TENANT_FORBIDDEN"
		}
		s.writeError(w, status, requestID, reason, resolveErr.Error())
		return
	}
	userJSON, err := ticketUserJSON(ticket)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "SSO_USER_INVALID", err.Error())
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
	if existing, lookupErr := s.store.User(agent.ID, ticket.Subject); lookupErr == nil {
		if existing.Status != "active" {
			s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_DISABLED", "agent user is disabled")
			return
		}
		if email, displayName, _ := userJSONFields(userJSON); email != "" || displayName != "" {
			_, _ = s.store.UpsertUser(agent.ID, ticket.Subject, email, displayName)
		}
	} else if errors.Is(lookupErr, errNotFound) {
		if _, err := s.store.UpsertUser(agent.ID, ticket.Subject, ticket.Email, ticket.DisplayName); err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to create SSO user mapping")
			return
		}
	} else {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load SSO user mapping")
		return
	}
	sessionID, err := s.store.CreateTenantSession(agent.ID, ticket.Subject, role, userJSON, "", "", time.Now().UTC().Add(sessionTTL))
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
	AgentID     string
	AgentRole   string
	AgentName   string
	JTI         string
	IssuedAt    time.Time
	ExpiresAt   time.Time
	Next        string
}

var errInvalidSSOTicket = errors.New("invalid SSO ticket")
var errSSOTenantForbidden = errors.New("SSO tenant claim is forbidden")

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
	agentID := strings.TrimSpace(stringValue(payload["agent_id"]))
	agentRole := strings.TrimSpace(stringValue(payload["agent_role"]))
	agentName := strings.TrimSpace(stringValue(payload["agent_name"]))
	if agentID == "" {
		if agentRole != "" || agentName != "" {
			return ssoTicket{}, errInvalidSSOTicket
		}
	} else if !validSharedAgentID(agentID) || agentRole != tenantRoleOwner && agentRole != tenantRoleMember || !validSharedAgentName(agentName) {
		return ssoTicket{}, errInvalidSSOTicket
	}
	return ssoTicket{
		Subject: subject, Email: stringValue(payload["email"]),
		DisplayName: firstNonEmpty(stringValue(payload["displayName"]), stringValue(payload["username"])),
		AvatarURL:   stringValue(payload["avatarUrl"]), AgentID: agentID,
		AgentRole: agentRole, AgentName: agentName, JTI: jti,
		IssuedAt: time.Unix(issuedAt, 0).UTC(), ExpiresAt: time.Unix(expiresAt, 0).UTC(), Next: next,
	}, nil
}

func validSharedAgentID(agentID string) bool {
	if len(agentID) != 36 || !strings.HasPrefix(agentID, "agt_") {
		return false
	}
	for _, value := range agentID[4:] {
		if value < '0' || value > '9' && (value < 'a' || value > 'f') {
			return false
		}
	}
	return true
}

func validSharedAgentName(name string) bool {
	if len([]rune(name)) > 100 {
		return false
	}
	for _, value := range name {
		if value < 0x20 || value == 0x7f {
			return false
		}
	}
	return true
}

func (s *Server) resolveSSOTicketAgent(r *http.Request, ticket ssoTicket) (AgentView, string, error) {
	if ticket.AgentID == "" {
		agent, err := s.requestAgent(r)
		if err != nil || agent.Status != "active" {
			return AgentView{}, "", fmt.Errorf("request host is not assigned to an active agent")
		}
		return agent, s.tenantRole(agent, ticket.Subject), nil
	}

	// Shared entry hosts select the tenant from the signed ticket/session, not
	// from persisted custom-domain rows. This also makes upgrades safe when an
	// old single-tenant row still claims the common host (for example localhost).
	if host, ok := normalizeTenantHost(r.Host); ok && !s.isSharedHost(r.Host) {
		mapped, err := s.store.AgentByDomain(host)
		if err == nil && mapped.ID != ticket.AgentID {
			return AgentView{}, "", fmt.Errorf("%w: request host belongs to another agent", errSSOTenantForbidden)
		}
		if err != nil && !errors.Is(err, errNotFound) {
			return AgentView{}, "", err
		}
	}

	agent, err := s.store.Agent(ticket.AgentID)
	if errors.Is(err, errNotFound) {
		if ticket.AgentRole != tenantRoleOwner {
			return AgentView{}, "", fmt.Errorf("%w: only an owner can create an AgentAPI tenant", errSSOTenantForbidden)
		}
		name := firstNonEmpty(ticket.AgentName, "AgentAPI")
		cfg := s.cfg
		cfg.AgentID = ticket.AgentID
		cfg.AgentDomain = ""
		cfg.AgentName = name
		cfg.SiteName = name
		cfg.SiteLogo = ""
		cfg.BrandSync = true
		cfg.AgentDisabled = false
		cfg.BillingMode = "user_upstream"
		cfg.OwnerMainUserID = ticket.Subject
		cfg.InitialBalanceCents = 0
		cfg.PaymentEnabled = false
		cfg.PaymentWebhookSecret = ""
		if err := s.store.UpsertAgent(cfg); err != nil {
			return AgentView{}, "", fmt.Errorf("create shared AgentAPI tenant: %w", err)
		}
		agent, err = s.store.Agent(ticket.AgentID)
	}
	if err != nil {
		return AgentView{}, "", err
	}
	if agent.Status != "active" {
		return AgentView{}, "", fmt.Errorf("AgentAPI tenant is not active")
	}
	if ticket.AgentRole == tenantRoleOwner && strings.TrimSpace(agent.OwnerMainUserID) != ticket.Subject {
		return AgentView{}, "", fmt.Errorf("%w: tenant owner does not match the signed subject", errSSOTenantForbidden)
	}
	return agent, ticket.AgentRole, nil
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
	agent, err := s.requestAgent(r)
	if err != nil || agent.Status != "active" {
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
	verifyCode := strings.TrimSpace(stringValue(payload["verify_code"]))
	affCode := strings.ToUpper(strings.TrimSpace(stringValue(payload["aff_code"])))
	if email == "" || password == "" {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "email and password are required")
		return
	}
	if affCode != "" && !validAffiliateCode(affCode) {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_AFFILIATE_CODE", "affiliate code must contain 4 to 32 letters, digits, underscores or dashes")
		return
	}

	// Create the authoritative user first. Local state is only membership;
	// never store passwords or accept role/balance/group fields from signup.
	if s.cfg.MainAdminAPIKey == "" {
		s.writeError(w, 503, requestID, "MAIN_ADMIN_NOT_CONFIGURED", "registration is unavailable")
		return
	}
	allowed, limitErr := s.store.AllowRegistration(agent.ID, registrationPeer(r), email)
	if limitErr != nil {
		s.writeError(w, 503, requestID, "REGISTRATION_UNAVAILABLE", "registration safety checks unavailable")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "3600")
		s.writeError(w, 429, requestID, "REGISTRATION_RATE_LIMITED", "too many registration attempts; try again later")
		return
	}
	if s.cfg.EmailVerifyEnabled {
		if verifyCode == "" || len(verifyCode) > 32 || strings.ContainsAny(verifyCode, "\r\n") {
			s.writeError(w, http.StatusBadRequest, requestID, "EMAIL_VERIFY_REQUIRED", "email verification is required")
			return
		}
		if err := s.main.VerifyRegistrationEmail(r.Context(), email, verifyCode, r.Header.Get("Accept-Language")); err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
	}
	marker, err := s.store.RegistrationMarker(agent.ID, email)
	if err != nil {
		s.writeError(w, 503, requestID, "REGISTRATION_UNAVAILABLE", "could not persist registration intent")
		return
	}
	createdUser, err := s.main.CreateMainUser(r.Context(), email, password, strings.TrimSpace(stringValue(payload["username"])), marker)
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	userID := mainUserIDFromJSON(createdUser)
	createdEmail, displayName, _ := userJSONFields(createdUser)
	if affCode != "" {
		if bindErr := s.main.BindAffiliateCode(r.Context(), userID, affCode); bindErr != nil {
			// Account creation is authoritative and cannot be rolled back from this
			// service. Preserve the new account/membership and make the failed bind
			// auditable instead of returning an error that encourages duplicate
			// registration attempts.
			s.recordTenantAudit(agent.ID, "system", agent.ID, "affiliate.bind", "main_user", userID, requestID, "failed", "main-site affiliate binding failed")
		} else {
			s.recordTenantAudit(agent.ID, "agent_user", userID, "affiliate.bind", "main_user", userID, requestID, "success", "")
		}
	}
	if _, err := s.store.UpsertUser(agent.ID, userID, createdEmail, displayName); err != nil {
		s.recordTenantAudit(agent.ID, "system", agent.ID, "user.register", "main_user", userID, requestID, "failed", "main user created; local membership needs repair")
		s.writeError(w, 503, requestID, "USER_MAPPING_FAILED", "main user created but local membership could not be saved; contact the site administrator")
		return
	}
	s.recordTenantAudit(agent.ID, "agent_user", userID, "user.register", "main_user", userID, requestID, "success", "")
	auth, err := s.main.Login(r.Context(), email, password)
	if err != nil {
		s.writeError(w, 503, requestID, "REGISTERED_LOGIN_REQUIRED", "account created; please sign in separately")
		return
	}
	if auth.Requires2FA {
		s.writeData(w, http.StatusOK, requestID, map[string]any{
			"requires_2fa": true, "temp_token": auth.TempToken,
		})
		return
	}
	if mainUserIDFromJSON(auth.User) != userID {
		s.writeError(w, 502, requestID, "UPSTREAM_IDENTITY_MISMATCH", "created account does not match the authenticated user")
		return
	}
	s.establishSession(w, r, requestID, auth, false)
}

func (s *Server) authSendVerifyCode(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	agent, err := s.requestAgent(r)
	if err != nil || agent.Status != "active" || s.cfg.MainAdminAPIKey == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "REGISTRATION_UNAVAILABLE", "registration is unavailable")
		return
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &payload, 16<<10); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
		return
	}
	payload.Email = strings.TrimSpace(payload.Email)
	if payload.Email == "" || len(payload.Email) > 320 || strings.ContainsAny(payload.Email, "\r\n") {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "a valid email address is required")
		return
	}
	if !s.cfg.EmailVerifyEnabled {
		s.writeError(w, http.StatusBadRequest, requestID, "EMAIL_VERIFY_DISABLED", "email verification is not enabled")
		return
	}
	data, err := s.main.SendRegistrationVerifyCode(r.Context(), payload.Email, r.Header.Get("Accept-Language"))
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	var result struct {
		Countdown int `json:"countdown"`
	}
	_ = json.Unmarshal(data, &result)
	countdown := result.Countdown
	if countdown <= 0 || countdown > 3600 {
		countdown = 60
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{
		"message": "Verification code sent successfully", "countdown": countdown,
	})
}

func validAffiliateCode(code string) bool {
	if len(code) < 4 || len(code) > 32 {
		return false
	}
	for _, char := range code {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
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

func (s *Server) authForgotPassword(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	var payload struct {
		Email                 string `json:"email"`
		TurnstileToken        string `json:"turnstile_token"`
		TencentCaptchaTicket  string `json:"tencent_captcha_ticket"`
		TencentCaptchaRandstr string `json:"tencent_captcha_randstr"`
	}
	if err := decodeJSON(r, &payload, 16<<10); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
		return
	}
	payload.Email = strings.TrimSpace(payload.Email)
	if payload.Email == "" || len(payload.Email) > 320 || strings.ContainsAny(payload.Email, "\r\n") {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "a valid email address is required")
		return
	}
	data, err := s.main.ForgotPassword(r.Context(), payload, r.Header.Get("Accept-Language"))
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil {
		result = map[string]any{}
	}
	message := strings.TrimSpace(stringValue(result["message"]))
	if message == "" {
		message = "If your email is registered, you will receive a password reset link shortly."
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"message": message})
}

func (s *Server) authPasswordRecoveryConfig(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	raw, err := s.main.PublicSettings(r.Context())
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	mainPublic := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &mainPublic); err != nil {
		s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_SETTINGS_INVALID", "main-site password recovery configuration is invalid")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{
		"password_reset_enabled":  rawBool(mainPublic["password_reset_enabled"]),
		"turnstile_enabled":       rawBool(mainPublic["turnstile_enabled"]),
		"turnstile_site_key":      rawString(mainPublic["turnstile_site_key"]),
		"tencent_captcha_enabled": rawBool(mainPublic["tencent_captcha_enabled"]),
		"tencent_captcha_app_id":  rawString(mainPublic["tencent_captcha_app_id"]),
		"tencent_captcha_region":  rawString(mainPublic["tencent_captcha_region"]),
		"aliyun_captcha_enabled":  rawBool(mainPublic["aliyun_captcha_enabled"]),
		"aliyun_captcha_scene_id": rawString(mainPublic["aliyun_captcha_scene_id"]),
		"aliyun_captcha_prefix":   rawString(mainPublic["aliyun_captcha_prefix"]),
		"aliyun_captcha_region":   rawString(mainPublic["aliyun_captcha_region"]),
	})
}

func (s *Server) authResetPassword(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	var payload struct {
		Email       string `json:"email"`
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &payload, 16<<10); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
		return
	}
	payload.Email = strings.TrimSpace(payload.Email)
	payload.Token = strings.TrimSpace(payload.Token)
	if payload.Email == "" || len(payload.Email) > 320 || strings.ContainsAny(payload.Email, "\r\n") || payload.Token == "" || len(payload.Token) > 2048 || len(payload.NewPassword) < 6 || len(payload.NewPassword) > 256 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "email, reset token, and a password of at least 6 characters are required")
		return
	}
	data, err := s.main.ResetPassword(r.Context(), payload, r.Header.Get("Accept-Language"))
	if err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil {
		result = map[string]any{}
	}
	message := strings.TrimSpace(stringValue(result["message"]))
	if message == "" {
		message = "Your password has been reset successfully. You can now log in with your new password."
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"message": message})
}

func (s *Server) establishSession(w http.ResponseWriter, r *http.Request, requestID string, auth MainAuthResult, createMapping bool) {
	agent, resolveErr := s.requestAgent(r)
	if resolveErr != nil || agent.Status != "active" {
		s.writeError(w, http.StatusMisdirectedRequest, requestID, "TENANT_NOT_FOUND", "request host is not assigned to an active agent")
		return
	}
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
	mappedUser, mapErr := s.store.User(agent.ID, mainUserID)
	if errors.Is(mapErr, errNotFound) {
		allowed := createMapping || s.cfg.AutoBindExistingUsers || mainUserID == agent.OwnerMainUserID
		if !allowed {
			recovered, recoveryErr := s.recoverRegistrationForTenant(r.Context(), agent.ID, mainUserID, email, displayName)
			if recoveryErr != nil {
				s.writeError(w, 503, requestID, "REGISTRATION_RECOVERY_UNAVAILABLE", "could not verify registration provenance; retry sign in later")
				return
			}
			allowed = recovered
		}
		if !allowed {
			s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_NOT_MAPPED", "this main-site account is not registered on this agent")
			return
		}
		if _, err := s.store.UpsertUser(agent.ID, mainUserID, email, displayName); err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to create agent user mapping")
			return
		}
	} else if mapErr != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load agent user mapping")
		return
	} else if mappedUser.Status != "active" {
		s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_DISABLED", "agent user is disabled")
		return
	}
	expiresAt := time.Now().UTC().Add(sessionTTL)
	sessionID, err := s.store.CreateTenantSession(agent.ID, mainUserID, s.tenantRole(agent, mainUserID), userJSON, auth.AccessToken, auth.RefreshToken, expiresAt)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "SESSION_CREATE_FAILED", "failed to create session")
		return
	}
	s.setSessionCookie(w, sessionID, expiresAt)
	s.writeData(w, http.StatusOK, requestID, s.browserAuthResponseForTenant(userJSON, mainUserID, agent))
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
			tenant, tenantOK := s.tenantContextFromSession(session)
			if tenantOK {
				_, _ = s.store.UpsertUser(tenant.AgentID, session.MainUserID, email, display)
			}
		}
	}
	tenant, tenantOK := s.tenantContextFromSession(session)
	if !tenantOK {
		s.writeError(w, http.StatusUnauthorized, requestID, "TENANT_CONTEXT_INVALID", "session tenant is invalid")
		return
	}
	agent, agentErr := s.store.Agent(tenant.AgentID)
	if agentErr != nil {
		s.writeError(w, http.StatusUnauthorized, requestID, "TENANT_CONTEXT_INVALID", "session tenant is unavailable")
		return
	}
	_ = user
	s.writeData(w, http.StatusOK, requestID, s.browserUserForTenant(current, session.MainUserID, agent))
}

func (s *Server) authRefresh(w http.ResponseWriter, r *http.Request, requestID string) {
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
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
	if strings.TrimSpace(auth.AccessToken) == "" {
		_ = s.store.DeleteSession(session.ID)
		s.clearSessionCookie(w)
		s.writeError(w, http.StatusBadGateway, requestID, "UPSTREAM_AUTH_INVALID", "main site did not return an access token")
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
	if err := validateMainSessionIdentity(current, session.MainUserID); err != nil {
		// A rotated credential must never replace the identity bound to this
		// AgentAPI session. Revoke the local session rather than returning a
		// profile for a different Sub2API user.
		_ = s.store.DeleteSession(session.ID)
		s.clearSessionCookie(w)
		s.writeMainError(w, requestID, err)
		return
	}
	expiresAt := time.Now().UTC().Add(sessionTTL)
	if err := s.store.UpdateSession(session.ID, session.MainUserID, current, auth.AccessToken, auth.RefreshToken, expiresAt); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "SESSION_UPDATE_FAILED", "failed to refresh session")
		return
	}
	agent, err := s.store.Agent(tenant.AgentID)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, requestID, "TENANT_CONTEXT_INVALID", "session tenant is unavailable")
		return
	}
	s.setSessionCookie(w, session.ID, expiresAt)
	s.writeData(w, http.StatusOK, requestID, map[string]any{"expires_in": int(sessionTTL.Seconds()), "token_type": "Cookie", "user": s.browserUserForTenant(current, session.MainUserID, agent)})
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
		if err := validateMainSessionIdentity(current, session.MainUserID); err != nil {
			return nil, err
		}
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
	if strings.TrimSpace(auth.AccessToken) == "" {
		return nil, &MainAPIError{Status: http.StatusBadGateway, Code: "UPSTREAM_AUTH_INVALID", Message: "main site did not return an access token"}
	}
	current, currentErr := s.main.CurrentUser(ctx, auth.AccessToken)
	if currentErr != nil {
		return nil, currentErr
	}
	if err := validateMainSessionIdentity(current, session.MainUserID); err != nil {
		return nil, err
	}
	// Rotating upstream credentials is not a local session renewal. This
	// helper cannot renew the browser cookie, so preserve its bound expiry.
	if err := s.store.UpdateSession(session.ID, session.MainUserID, current, auth.AccessToken, auth.RefreshToken, session.ExpiresAt); err != nil {
		return nil, fmt.Errorf("refresh AgentAPI session credentials: %w", err)
	}
	return current, nil
}

func validateMainSessionIdentity(userJSON []byte, expectedMainUserID string) error {
	actualMainUserID := mainUserIDFromJSON(userJSON)
	if strings.TrimSpace(expectedMainUserID) == "" || actualMainUserID == "" || actualMainUserID != expectedMainUserID {
		return &MainAPIError{Status: http.StatusUnauthorized, Code: "SESSION_IDENTITY_MISMATCH", Message: "main-site session identity no longer matches this AgentAPI session"}
	}
	return nil
}

func (s *Server) handlePublicSettings(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	branding, err := s.requestAgent(r)
	if err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusMisdirectedRequest, requestID, "TENANT_NOT_FOUND", "request host is not assigned to an agent")
			return
		}
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
	payment := s.paymentConfigFor(branding.ID)
	paymentEnabled := payment.Enabled && branding.BillingMode != "user_upstream"
	rechargeURL := ""
	if s.cfg.PublicMainURL != "" {
		rechargeURL = strings.TrimRight(s.cfg.PublicMainURL, "/") + "/purchase"
	}
	emailVerifyEnabled := s.cfg.EmailVerifyEnabled
	registrationEnabled := s.cfg.MainAdminAPIKey != "" && branding.Status == "active" && (!emailVerifyEnabled || strings.TrimSpace(s.cfg.AppCredential) != "")
	settings := map[string]any{
		"registration_enabled": registrationEnabled, "email_verify_enabled": emailVerifyEnabled,
		"main_site_sso_enabled":             func() bool { _, ok := publicMainOrigin(s.cfg.PublicMainURL); return ok }(),
		"force_email_on_third_party_signup": false, "registration_email_suffix_whitelist": []string{},
		"registration_email_domain_quota_enabled": false, "promo_code_enabled": false,
		"password_reset_enabled": false, "invitation_code_enabled": false,
		"login_agreement_enabled": false, "turnstile_enabled": false,
		"turnstile_site_key":      "",
		"tencent_captcha_enabled": false,
		"tencent_captcha_app_id":  "",
		"tencent_captcha_region":  "",
		"aliyun_captcha_enabled":  false,
		"aliyun_captcha_scene_id": "",
		"aliyun_captcha_prefix":   "",
		"aliyun_captcha_region":   "",
		// Passkey availability depends on the current browser origin and is
		// therefore served by /auth/passkey/config. Keep this legacy settings
		// response local-only so public branding never depends on Sub2API.
		"passkey_enabled":    false,
		"passkey_configured": false,
		"site_name":          siteName, "site_logo": logo, "site_subtitle": branding.SiteSubtitle,
		"api_base_url": branding.APIBaseURL, "contact_info": branding.ContactInfo, "doc_url": branding.DocURL, "home_content": branding.HomeContent,
		"recharge_url":         rechargeURL,
		"compact_home_enabled": branding.CompactHomeEnabled, "hide_ccs_import_button": true,
		// The public flag only tells the UI whether this AgentAPI instance has
		// enabled its signed webhook bridge. It never exposes a secret or a main
		// site credential. A verified payment still requires synced owner credit.
		"payment_enabled": paymentEnabled, "payment_provider": payment.Provider,
		"payment_currency": payment.Currency, "payment_min_amount_cents": payment.MinAmountCents,
		"payment_max_amount_cents": payment.MaxAmountCents, "payment_balance_disabled": true,
		"risk_control_enabled": false, "table_default_page_size": 20, "table_page_size_options": []int{20, 50, 100},
		"custom_menu_items": []any{}, "custom_endpoints": []any{},
		"linuxdo_oauth_enabled": false, "wechat_oauth_enabled": false, "oidc_oauth_enabled": false,
		"oidc_oauth_provider_name": "", "github_oauth_enabled": false, "google_oauth_enabled": false,
		"backend_mode_enabled": false, "version": "agentapi", "balance_low_notify_enabled": false,
		"account_quota_notify_enabled": false, "balance_low_notify_threshold": 0,
		"channel_monitor_enabled": false, "channel_monitor_default_interval_seconds": 60,
		"available_channels_enabled": false, "subscription_enabled": false, "model_plaza_enabled": false,
		"model_plaza_require_auth": true, "plugin_management_enabled": false, "service_quota_enabled": false,
		"affiliate_enabled": true, "allow_user_view_error_requests": false,
	}
	s.writeData(w, http.StatusOK, requestID, settings)
}

func (s *Server) handleAgentContext(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	agent, err := s.requestAgent(r)
	if err != nil {
		s.writeError(w, http.StatusMisdirectedRequest, requestID, "TENANT_NOT_FOUND", "request host is not assigned to an agent")
		return
	}
	// Branding and readiness are public; owner balance and allocation totals are
	// only visible to the configured agent administrator.
	publicAgent := redactAgentFinancials(agent)
	result := map[string]any{"agent": publicAgent, "authenticated": false, "is_agent_admin": false}
	if session, user, ok := s.loadSession(r); ok {
		if mainUser, balanceErr := s.main.AdminGetUser(r.Context(), session.MainUserID); balanceErr == nil {
			user.BalanceCents = mainUser.Balance
			user.FrozenBalanceCents = mainUser.FrozenBalance
		} else {
			result["balance_error"] = "Sub2API user balance is temporarily unavailable"
		}
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

func (s *Server) handleAgentProfile(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		// SSO-created sessions intentionally carry identity only. They can view
		// the profile claims stored at sign-in, but need an authenticated main-
		// site session to edit account details.
		profile := append(json.RawMessage(nil), session.UserJSON...)
		if strings.TrimSpace(session.AccessToken) != "" {
			if _, err := s.currentSessionUser(r.Context(), session); err != nil {
				s.writeMainError(w, requestID, err)
				return
			}
			refreshedSession, err := s.store.LoadSession(session.ID)
			if err != nil {
				s.writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "AgentAPI session is no longer valid")
				return
			}
			session = refreshedSession
			current, err := s.main.UserProfile(r.Context(), session.AccessToken)
			if err != nil {
				s.writeMainError(w, requestID, err)
				return
			}
			profile = current
		}
		email, username, _ := userJSONFields(profile)
		if email != "" || username != "" {
			_, _ = s.store.UpsertUser(tenant.AgentID, session.MainUserID, email, username)
		}
		s.writeData(w, http.StatusOK, requestID, safeAgentProfile(profile, session.MainUserID, strings.TrimSpace(session.AccessToken) != ""))
	case http.MethodPut:
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		if strings.TrimSpace(session.AccessToken) == "" {
			s.writeError(w, http.StatusForbidden, requestID, "MAIN_SESSION_REQUIRED", "sign in with your main-site account to edit your profile")
			return
		}
		var payload struct {
			Username string `json:"username"`
		}
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
			return
		}
		payload.Username = strings.TrimSpace(payload.Username)
		if payload.Username == "" || len(payload.Username) > 128 || strings.ContainsAny(payload.Username, "\r\n\x00") {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_USERNAME", "username must be non-empty, at most 128 bytes and contain no line breaks")
			return
		}
		if _, err := s.currentSessionUser(r.Context(), session); err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		refreshedSession, err := s.store.LoadSession(session.ID)
		if err != nil {
			s.writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "AgentAPI session is no longer valid")
			return
		}
		session = refreshedSession
		profile, err := s.main.UpdateUserProfile(r.Context(), session.AccessToken, payload.Username)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		email, username, _ := userJSONFields(profile)
		if current, loadErr := s.store.User(tenant.AgentID, session.MainUserID); loadErr == nil {
			if email == "" {
				email = current.Email
			}
			if username == "" {
				username = payload.Username
			}
		}
		if _, err := s.store.UpsertUser(tenant.AgentID, session.MainUserID, email, username); err != nil {
			// Sub2API is authoritative for profile data. A local display-name
			// mirror failure must not turn a committed upstream update into an
			// apparent failure that encourages a misleading retry.
			slog.Error("failed to refresh Agent user display name after main profile update", "agent_id", tenant.AgentID, "main_user_id", session.MainUserID, "request_id", requestID, "error", err)
		}
		s.writeData(w, http.StatusOK, requestID, safeAgentProfile(profile, session.MainUserID, true))
	default:
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (s *Server) handleAgentPassword(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !sameOrigin(r) {
		s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
		return
	}
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}
	if strings.TrimSpace(session.AccessToken) == "" {
		s.writeError(w, http.StatusForbidden, requestID, "MAIN_SESSION_REQUIRED", "sign in with your main-site account to change your password")
		return
	}
	var payload struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
		return
	}
	if payload.OldPassword == "" || len(payload.NewPassword) < 6 || len(payload.NewPassword) > 1024 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PASSWORD", "current password is required and new password must contain at least 6 bytes")
		return
	}
	if _, err := s.currentSessionUser(r.Context(), session); err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	refreshedSession, err := s.store.LoadSession(session.ID)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "AgentAPI session is no longer valid")
		return
	}
	session = refreshedSession
	if err := s.main.ChangeUserPassword(r.Context(), session.AccessToken, payload.OldPassword, payload.NewPassword); err != nil {
		s.writeMainError(w, requestID, err)
		return
	}
	_ = s.store.DeleteSession(session.ID)
	s.clearSessionCookie(w)
	s.recordTenantAudit(tenant.AgentID, "user", session.MainUserID, "password_change", "user", session.MainUserID, requestID, "success", "")
	s.writeData(w, http.StatusOK, requestID, map[string]string{"message": "password changed; sign in again"})
}

func safeAgentProfile(raw []byte, mainUserID string, canEdit bool) map[string]any {
	profile := map[string]any{
		"id":       mainUserID,
		"email":    "",
		"username": "",
		"can_edit": canEdit,
	}
	var upstream map[string]json.RawMessage
	if json.Unmarshal(raw, &upstream) == nil {
		for _, key := range []string{"email", "username", "avatar_url"} {
			var value string
			if json.Unmarshal(upstream[key], &value) == nil {
				profile[key] = value
			}
		}
		if profile["username"] == "" {
			var displayName string
			if json.Unmarshal(upstream["display_name"], &displayName) == nil {
				profile["username"] = displayName
			}
		}
	}
	return profile
}

func (s *Server) handleAgentWallet(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant, session, user, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}
	agent, err := s.store.Wallet(tenant.AgentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load wallet")
		return
	}
	if !s.isAgentAdmin(session) {
		agent = redactAgentFinancials(agent)
	}
	mainUser, balanceErr := s.main.AdminGetUser(r.Context(), session.MainUserID)
	if balanceErr != nil {
		s.writeMainError(w, requestID, balanceErr)
		return
	}
	user.BalanceCents = mainUser.Balance
	s.writeData(w, http.StatusOK, requestID, map[string]any{"agent": agent, "user": user, "main_user_id": session.MainUserID})
}

func agentPagination(r *http.Request) (int, int, error) {
	page, pageSize := 1, 25
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1_000_000_000 {
			return 0, 0, fmt.Errorf("page must be between 1 and 1000000000")
		}
		page = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			return 0, 0, fmt.Errorf("page_size must be between 1 and 200")
		}
		pageSize = parsed
	}
	return page, pageSize, nil
}

func (s *Server) handleAgentUsage(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}
	mainUserID := session.MainUserID
	if s.isAgentAdmin(session) {
		mainUserID = ""
	}
	page, pageSize := 1, 25
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1_000_000_000 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PAGINATION", "page must be between 1 and 1000000000")
			return
		}
		page = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PAGINATION", "page_size must be between 1 and 200")
			return
		}
		pageSize = parsed
	}
	search, err := parseUsageFilter(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_FILTER", err.Error())
		return
	}
	items, total, err := s.store.FilteredUsagePage(tenant.AgentID, mainUserID, pageSize, (page-1)*pageSize, search)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load usage")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize})
}

// handleAgentRecharge owns the user-facing order lifecycle. It deliberately
// does not accept agent_id, main_user_id, provider, or currency from the
// browser: all of those values come from the authenticated session and the
// instance configuration.
func (s *Server) handleAgentRecharge(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}
	agent, err := s.store.Agent(tenant.AgentID)
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "TENANT_CONTEXT_INVALID", "session tenant is unavailable")
		return
	}
	if s.tenantBillingMode(agent) == "user_upstream" {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusGone, requestID, "LOCAL_RECHARGE_DISABLED", "recharge is completed on the Sub2API main site")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{
			"enabled": false, "provider": "sub2api", "currency": "", "items": []RechargeOrder{}, "total": 0,
			"recharge_url": strings.TrimRight(s.cfg.PublicMainURL, "/") + "/purchase",
		})
		return
	}
	payment := s.paymentConfigFor(tenant.AgentID)
	switch r.Method {
	case http.MethodGet:
		if err := s.store.ExpireRechargeOrders(tenant.AgentID); err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to expire recharge orders")
			return
		}
		orders, err := s.store.RechargeOrders(tenant.AgentID, session.MainUserID, 100)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load recharge orders")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{
			"enabled":          payment.Enabled,
			"provider":         payment.Provider,
			"currency":         payment.Currency,
			"min_amount_cents": payment.MinAmountCents,
			"max_amount_cents": payment.MaxAmountCents,
			"items":            orders,
			"total":            len(orders),
		})
		return
	case http.MethodPost:
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		if !payment.Enabled {
			s.writeError(w, http.StatusGone, requestID, "PAYMENT_DISABLED", "recharge is not enabled for this AgentAPI instance")
			return
		}
		var payload map[string]any
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
			return
		}
		amountCents, err := amountFromPayload(payload)
		if err != nil || amountCents < payment.MinAmountCents || amountCents > payment.MaxAmountCents {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_AMOUNT", fmt.Sprintf("amount must be between %s and %s %s", formatCents(payment.MinAmountCents), formatCents(payment.MaxAmountCents), payment.Currency))
			return
		}
		requestKey := requestIDFrom(r, payload)
		provider := strings.TrimSpace(payment.Provider)
		currency := strings.ToUpper(strings.TrimSpace(payment.Currency))
		expiresAt := time.Now().UTC().Add(time.Duration(payment.OrderTTLSeconds) * time.Second)
		order, created, err := s.store.CreateRechargeOrder(tenant.AgentID, session.MainUserID, amountCents, currency, provider, "", requestKey, expiresAt)
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
		if created && strings.TrimSpace(payment.CheckoutURLTemplate) != "" {
			checkoutURL := paymentCheckoutURL(payment.CheckoutURLTemplate, payment.MerchantID, order)
			if updated, updateErr := s.store.SetRechargePaymentURL(tenant.AgentID, order.OrderNo, checkoutURL); updateErr == nil {
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
	agent, err := s.requestAgent(r)
	if err != nil || agent.Status != "active" {
		s.writeError(w, http.StatusNotFound, requestID, "AGENT_NOT_FOUND", "agent tenant was not found")
		return
	}
	if s.tenantBillingMode(agent) == "user_upstream" {
		s.writeError(w, http.StatusGone, requestID, "LOCAL_RECHARGE_DISABLED", "recharge is completed on the Sub2API main site")
		return
	}
	payment := s.paymentConfigFor(agent.ID)
	if !payment.Enabled {
		s.writeError(w, http.StatusNotFound, requestID, "PAYMENT_DISABLED", "payment webhook is disabled")
		return
	}
	secret := strings.TrimSpace(payment.WebhookSecret)
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
		MerchantID      string `json:"merchant_id"`
		ProviderTradeNo string `json:"provider_trade_no"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "payment webhook body must be valid JSON")
		return
	}
	payload.OrderNo = strings.TrimSpace(payload.OrderNo)
	payload.Status = strings.ToLower(strings.TrimSpace(payload.Status))
	payload.Currency = strings.ToUpper(strings.TrimSpace(payload.Currency))
	payload.MerchantID = strings.TrimSpace(payload.MerchantID)
	payload.ProviderTradeNo = strings.TrimSpace(payload.ProviderTradeNo)
	if payload.OrderNo == "" || payload.AmountCents <= 0 || payload.Currency == "" || payload.ProviderTradeNo == "" || len(payload.ProviderTradeNo) > 256 {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PAYMENT_EVENT", "order_no, status, amount_cents, currency and provider_trade_no are required")
		return
	}
	if payment.MerchantID != "" && payload.MerchantID != payment.MerchantID {
		s.writeError(w, http.StatusBadRequest, requestID, "PAYMENT_MERCHANT_MISMATCH", "payment event merchant_id does not match this instance")
		return
	}
	if err := s.store.ExpireRechargeOrders(agent.ID); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to expire recharge orders")
		return
	}
	payloadHash := sha256.Sum256(body)
	result, err := s.store.RecordPaymentEvent(agent.ID, payment.Provider, eventID, payload.OrderNo, payload.Status, payload.AmountCents, payload.Currency, payload.ProviderTradeNo, hex.EncodeToString(payloadHash[:]))
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
		if allocatedOrder, allocateErr := s.syncAndAllocateRechargeOrderForTenant(r.Context(), agent.ID, requestID+":payment-sync", order.OrderNo); allocateErr != nil {
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

func (s *Server) paymentAdminOrders(w http.ResponseWriter, r *http.Request, requestID, agentID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if err := s.store.ExpireRechargeOrders(agentID); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to expire recharge orders")
		return
	}
	orders, err := s.store.RechargeOrders(agentID, "", 500)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load recharge orders")
		return
	}
	s.writeData(w, http.StatusOK, requestID, map[string]any{"enabled": s.paymentConfigFor(agentID).Enabled, "items": orders, "total": len(orders)})
}

func (s *Server) paymentAdminAllocate(w http.ResponseWriter, r *http.Request, requestID, agentID, actorID string) {
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
	if err := s.store.ExpireRechargeOrders(agentID); err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to expire recharge orders")
		return
	}
	order, err := s.store.RechargeOrder(agentID, orderNo)
	if err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusNotFound, requestID, "RECHARGE_ORDER_NOT_FOUND", "recharge order not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load recharge order")
		return
	}
	if order.Status == "allocated" {
		s.recordTenantAudit(agentID, "agent_admin", actorID, "recharge.allocate", "recharge_order", orderNo, requestID, "success", "idempotent replay")
		s.writeData(w, http.StatusOK, requestID, map[string]any{"order": order, "allocated": true})
		return
	}
	if order.Status != "paid_pending_allocation" {
		s.writeError(w, http.StatusConflict, requestID, "RECHARGE_NOT_PAID", "only paid_pending_allocation orders can be allocated")
		return
	}
	order, err = s.syncAndAllocateRechargeOrderForTenant(r.Context(), agentID, requestID+":payment-admin-sync", orderNo)
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
	s.recordTenantAudit(agentID, "agent_admin", actorID, "recharge.allocate", "recharge_order", orderNo, requestID, "success", "")
	s.writeData(w, http.StatusOK, requestID, map[string]any{"order": order, "allocated": order.Status == "allocated"})
}

func paymentCheckoutURL(template, merchantID string, order RechargeOrder) string {
	return strings.NewReplacer(
		"{order_no}", url.QueryEscape(order.OrderNo),
		"{amount}", formatCents(order.AmountCents),
		"{amount_cents}", strconv.FormatInt(order.AmountCents, 10),
		"{currency}", url.QueryEscape(order.Currency),
		"{merchant_id}", url.QueryEscape(strings.TrimSpace(merchantID)),
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

func validateBrandDocURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 2048 || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("doc_url must be at most 2048 bytes and contain no line breaks")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("doc_url must be an http or https URL")
	}
	return nil
}

func validateAPIBaseURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 2048 || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("api_base_url must be at most 2048 bytes and contain no line breaks")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("api_base_url must be an http or https URL without credentials, query, or fragment")
	}
	return nil
}

func validateBrandContactInfo(value string) error {
	value = strings.TrimSpace(value)
	if len(value) > 300 {
		return fmt.Errorf("contact_info must be at most 300 bytes")
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("contact_info contains a control character")
		}
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

func (s *Server) tenantBillingMode(agent AgentView) string {
	mode := strings.TrimSpace(agent.BillingMode)
	if mode == "" {
		return "user_upstream"
	}
	return mode
}

func (s *Server) paymentConfigFor(agentID string) PaymentConfig {
	config, err := s.store.PaymentConfig(agentID)
	if err == nil {
		return config
	}
	slog.Error("failed to load instance payment config", "agent_id", agentID, "error", err)
	return PaymentConfig{
		Enabled:             false,
		Provider:            firstNonEmpty(strings.TrimSpace(s.cfg.PaymentProvider), "manual"),
		Currency:            firstNonEmpty(strings.ToUpper(strings.TrimSpace(s.cfg.PaymentCurrency)), "CNY"),
		MinAmountCents:      s.cfg.PaymentMinCents,
		MaxAmountCents:      s.cfg.PaymentMaxCents,
		OrderTTLSeconds:     int64(s.cfg.PaymentOrderTTL / time.Second),
		CheckoutURLTemplate: strings.TrimSpace(s.cfg.PaymentCheckoutURLTemplate),
	}
}

func validatePaymentConfig(config PaymentConfig, effectiveSecret string) error {
	if config.Provider == "" || len(config.Provider) > 64 {
		return fmt.Errorf("provider must contain 1 to 64 characters")
	}
	for _, r := range config.Provider {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
			return fmt.Errorf("provider may contain only letters, numbers, dot, underscore and hyphen")
		}
	}
	if len(config.Currency) != 3 {
		return fmt.Errorf("currency must be a three-letter code")
	}
	for _, r := range config.Currency {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("currency must be a three-letter uppercase code")
		}
	}
	if len(config.MerchantID) > 256 || strings.ContainsAny(config.MerchantID, "\r\n\x00") {
		return fmt.Errorf("merchant_id must be at most 256 bytes and contain no line breaks")
	}
	if config.MinAmountCents <= 0 || config.MaxAmountCents < config.MinAmountCents || config.MaxAmountCents > 100000000000 {
		return fmt.Errorf("payment amount limits are invalid")
	}
	if config.OrderTTLSeconds < 60 || config.OrderTTLSeconds > 7*24*60*60 {
		return fmt.Errorf("order_ttl_seconds must be between 60 and 604800")
	}
	if len(config.CheckoutURLTemplate) > 4096 || strings.ContainsAny(config.CheckoutURLTemplate, "\r\n\x00") {
		return fmt.Errorf("checkout_url_template must be at most 4096 bytes and contain no line breaks")
	}
	if config.CheckoutURLTemplate != "" {
		preview := paymentCheckoutURL(config.CheckoutURLTemplate, config.MerchantID, RechargeOrder{OrderNo: "preview", AmountCents: config.MinAmountCents, Currency: config.Currency})
		parsed, err := url.Parse(preview)
		if err != nil || parsed.Host == "" || parsed.Scheme != "https" && parsed.Scheme != "http" {
			return fmt.Errorf("checkout_url_template must produce an http or https URL")
		}
	}
	if config.Enabled && strings.TrimSpace(effectiveSecret) == "" {
		return fmt.Errorf("webhook_secret is required when payments are enabled")
	}
	return nil
}

func (s *Server) paymentAdminConfig(w http.ResponseWriter, r *http.Request, requestID, agentID, actorID string) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		config := s.paymentConfigFor(agentID)
		config.WebhookSecret = ""
		s.writeData(w, http.StatusOK, requestID, config)
	case http.MethodPut, http.MethodPatch:
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		var payload struct {
			Enabled             bool    `json:"enabled"`
			Provider            string  `json:"provider"`
			Currency            string  `json:"currency"`
			MerchantID          string  `json:"merchant_id"`
			WebhookSecret       *string `json:"webhook_secret"`
			ClearWebhookSecret  bool    `json:"clear_webhook_secret"`
			MinAmountCents      int64   `json:"min_amount_cents"`
			MaxAmountCents      int64   `json:"max_amount_cents"`
			OrderTTLSeconds     int64   `json:"order_ttl_seconds"`
			CheckoutURLTemplate string  `json:"checkout_url_template"`
		}
		if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
			return
		}
		current := s.paymentConfigFor(agentID)
		replaceSecret := payload.WebhookSecret != nil || payload.ClearWebhookSecret
		secret := current.WebhookSecret
		if payload.WebhookSecret != nil {
			secret = strings.TrimSpace(*payload.WebhookSecret)
		}
		if payload.ClearWebhookSecret {
			secret = ""
		}
		config := PaymentConfig{
			Enabled:             payload.Enabled,
			Provider:            strings.TrimSpace(payload.Provider),
			Currency:            strings.ToUpper(strings.TrimSpace(payload.Currency)),
			MerchantID:          strings.TrimSpace(payload.MerchantID),
			WebhookSecret:       secret,
			MinAmountCents:      payload.MinAmountCents,
			MaxAmountCents:      payload.MaxAmountCents,
			OrderTTLSeconds:     payload.OrderTTLSeconds,
			CheckoutURLTemplate: strings.TrimSpace(payload.CheckoutURLTemplate),
		}
		if err := validatePaymentConfig(config, secret); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PAYMENT_CONFIG", err.Error())
			return
		}
		updated, err := s.store.UpdatePaymentConfig(agentID, config, replaceSecret)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "PAYMENT_CONFIG_UPDATE_FAILED", "failed to save payment configuration")
			return
		}
		updated.WebhookSecret = ""
		s.recordTenantAudit(agentID, "agent_admin", actorID, "payment_config.update", "agent", agentID, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, updated)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported payment configuration operation")
	}
}

func (s *Server) handleAgentAdmin(w http.ResponseWriter, r *http.Request, requestID string) {
	if isForbiddenAgentManagementPath(r.URL.Path) {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "agent admin endpoint not found")
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/wallet/credit" {
		// There is intentionally no local top-up endpoint in owner_upstream mode.
		// Main-site payments change the owner's balance; AgentAPI can only read
		// that balance through the server-side administrator credential.
		s.writeError(w, http.StatusGone, requestID, "LOCAL_CREDIT_DISABLED", "agent balance is funded on the main site")
		return
	}

	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok || !s.isAgentAdmin(session) {
		if ok {
			s.writeError(w, http.StatusForbidden, requestID, "AGENT_ADMIN_REQUIRED", "agent administrator access required")
		}
		return
	}
	tenantAgent, err := s.store.Agent(tenant.AgentID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load tenant configuration")
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/announcements" || strings.HasPrefix(r.URL.Path, "/api/v1/agent/admin/announcements/") {
		s.handleAgentAdminAnnouncements(w, r, requestID, tenant, session)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/users" {
		s.handleAgentAdminUsers(w, r, requestID, tenant)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/payment-config" {
		if s.tenantBillingMode(tenantAgent) == "user_upstream" {
			s.writeError(w, http.StatusGone, requestID, "LOCAL_RECHARGE_DISABLED", "local payment configuration is disabled for direct user billing")
			return
		}
		s.paymentAdminConfig(w, r, requestID, tenant.AgentID, session.MainUserID)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/branding" {
		switch r.Method {
		case http.MethodGet:
			s.writeData(w, http.StatusOK, requestID, agentBrandingData(tenantAgent))
			return
		case http.MethodPut, http.MethodPatch:
			if !sameOrigin(r) {
				s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
				return
			}
			var payload struct {
				Name               *string `json:"name"`
				SiteName           *string `json:"site_name"`
				SiteLogo           *string `json:"site_logo"`
				DocURL             *string `json:"doc_url"`
				ContactInfo        *string `json:"contact_info"`
				APIBaseURL         *string `json:"api_base_url"`
				SiteSubtitle       *string `json:"site_subtitle"`
				CompactHomeEnabled *bool   `json:"compact_home_enabled"`
				HomeContent        *string `json:"home_content"`
			}
			if err := decodeJSON(r, &payload, maxJSONBody); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error())
				return
			}
			current := tenantAgent
			name, siteName, siteLogo, docURL, contactInfo, apiBaseURL := current.Name, current.SiteName, current.SiteLogo, current.DocURL, current.ContactInfo, current.APIBaseURL
			home := current.AgentHomeSettings
			if payload.SiteSubtitle != nil {
				home.SiteSubtitle = strings.TrimSpace(*payload.SiteSubtitle)
			}
			if payload.CompactHomeEnabled != nil {
				home.CompactHomeEnabled = *payload.CompactHomeEnabled
			}
			if payload.HomeContent != nil {
				home.HomeContent = strings.TrimSpace(*payload.HomeContent)
			}
			if payload.Name != nil {
				name = strings.TrimSpace(*payload.Name)
			}
			if payload.SiteName != nil {
				siteName = strings.TrimSpace(*payload.SiteName)
			}
			if payload.SiteLogo != nil {
				siteLogo = strings.TrimSpace(*payload.SiteLogo)
			}
			if payload.DocURL != nil {
				docURL = strings.TrimSpace(*payload.DocURL)
			}
			if payload.ContactInfo != nil {
				contactInfo = strings.TrimSpace(*payload.ContactInfo)
			}
			if payload.APIBaseURL != nil {
				apiBaseURL = strings.TrimRight(strings.TrimSpace(*payload.APIBaseURL), "/")
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
			if err := validateBrandDocURL(docURL); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BRANDING", err.Error())
				return
			}
			if err := validateBrandContactInfo(contactInfo); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BRANDING", err.Error())
				return
			}
			if err := validateAPIBaseURL(apiBaseURL); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BRANDING", err.Error())
				return
			}
			if err := validateAgentHomeSettings(home); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_BRANDING", err.Error())
				return
			}
			updated, err := s.store.UpdateBranding(tenant.AgentID, name, siteName, siteLogo, docURL, contactInfo, apiBaseURL, home)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "BRANDING_UPDATE_FAILED", "failed to save branding")
				return
			}
			s.recordTenantAudit(tenant.AgentID, "agent_admin", session.MainUserID, "branding.update", "agent", tenant.AgentID, requestID, "success", "")
			s.writeData(w, http.StatusOK, requestID, agentBrandingData(updated))
			return
		default:
			s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported branding operation")
			return
		}
	}
	if r.URL.Path == "/api/v1/agent/admin/wallet" && r.Method == http.MethodGet {
		agent, err := s.store.Agent(tenant.AgentID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load wallet")
			return
		}
		s.writeData(w, http.StatusOK, requestID, agent)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/recharge/orders" {
		if s.tenantBillingMode(tenantAgent) == "user_upstream" {
			s.writeError(w, http.StatusGone, requestID, "LOCAL_RECHARGE_DISABLED", "local recharge orders are disabled for direct user billing")
			return
		}
		s.paymentAdminOrders(w, r, requestID, tenant.AgentID)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/recharge/allocate" {
		if s.tenantBillingMode(tenantAgent) == "user_upstream" {
			s.writeError(w, http.StatusGone, requestID, "LOCAL_ALLOCATION_DISABLED", "users spend their own Sub2API balance")
			return
		}
		s.paymentAdminAllocate(w, r, requestID, tenant.AgentID, session.MainUserID)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/wallet/sync" && r.Method == http.MethodPost {
		if s.tenantBillingMode(tenantAgent) == "user_upstream" {
			s.writeError(w, http.StatusGone, requestID, "OWNER_WALLET_DISABLED", "direct user billing has no shared owner wallet")
			return
		}
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		agent, err := s.syncOwnerBalanceForTenant(r.Context(), tenant.AgentID, requestID)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.recordTenantAudit(tenant.AgentID, "agent_admin", session.MainUserID, "wallet.sync", "agent_wallet", tenant.AgentID, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, agent)
		return
	}
	if r.URL.Path == "/api/v1/agent/admin/settlements" && r.Method == http.MethodGet {
		items, err := s.store.PendingSettlements(tenant.AgentID, s.cfg.SettlementReconcileBatch)
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
		items, err := s.reconcileTenantSettlements(r.Context(), tenant.AgentID, requestIDFilter)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.recordTenantAudit(tenant.AgentID, "agent_admin", session.MainUserID, "settlements.reconcile", "settlement", requestIDFilter, requestID, "success", "")
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": items, "total": len(items)})
		return
	}
	s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "agent admin endpoint not found")
}

func (s *Server) handleAPIKeys(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	tenant, session, _, ok := s.requireTenantSession(w, r, requestID)
	if !ok {
		return
	}

	keyUserID := session.MainUserID
	actorType := "agent_user"
	if s.isAgentAdmin(session) {
		actorType = "agent_admin"
	}
	if target := strings.TrimSpace(r.URL.Query().Get("main_user_id")); target != "" && target != keyUserID {
		s.writeError(w, http.StatusForbidden, requestID, "CROSS_USER_KEY_MANAGEMENT_DISABLED", "AgentAPI users can only manage their own keys")
		return
	}
	if r.URL.Path == "/api/v1/api-keys" && r.Method == http.MethodGet {
		keys, err := s.store.APIKeys(tenant.AgentID, keyUserID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to list AgentAPI keys")
			return
		}
		agent, err := s.store.Agent(tenant.AgentID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to load AgentAPI address")
			return
		}
		s.writeData(w, http.StatusOK, requestID, map[string]any{"items": keys, "total": len(keys), "api_base_url": agent.APIBaseURL})
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
		view, raw, err := s.store.CreateAPIKey(tenant.AgentID, keyUserID, name)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "API_KEY_CREATE_FAILED", "failed to create AgentAPI key")
			return
		}
		s.recordTenantAudit(tenant.AgentID, actorType, session.MainUserID, "api_key.create", "agent_api_key", strconv.FormatInt(view.ID, 10), requestID, "success", "key_user_id="+keyUserID)
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
		if err := s.store.RevokeAPIKey(tenant.AgentID, keyUserID, id); err != nil {
			if errors.Is(err, errNotFound) {
				s.writeError(w, http.StatusNotFound, requestID, "API_KEY_NOT_FOUND", "AgentAPI key not found")
				return
			}
			s.writeError(w, http.StatusInternalServerError, requestID, "API_KEY_REVOKE_FAILED", "failed to revoke AgentAPI key")
			return
		}
		s.recordTenantAudit(tenant.AgentID, actorType, session.MainUserID, "api_key.revoke", "agent_api_key", strconv.FormatInt(id, 10), requestID, "success", "key_user_id="+keyUserID)
		s.writeData(w, http.StatusOK, requestID, map[string]any{"id": id, "status": "revoked"})
		return
	}

	s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported AgentAPI key operation")
}

func (s *Server) handleModelRelay(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.URL.Path == "/v1/usage" {
		s.handleAPIKeyUsage(w, r, requestID)
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
	agentID := principal.Tenant.AgentID
	agent, err := s.store.Agent(agentID)
	if err != nil || agent.Status != "active" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "AGENT_SUSPENDED", "agent is not active for model requests")
		return
	}
	billingMode := s.tenantBillingMode(agent)
	if r.URL.Path == "/v1/models" {
		// Do not proxy the main site's complete model inventory. It may contain
		// private/provider-specific names that are outside the satellite public
		// catalog and would let clients discover an unintended route.
		s.writePublicModels(w)
		return
	}
	if r.Method == http.MethodGet {
		if strings.HasPrefix(r.URL.Path, "/v1/images/tasks/") {
			s.handleImageTaskPoll(w, r, requestID, principal)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/videos/") {
			s.handleVideoPoll(w, r, requestID, principal)
			return
		}
		status, headers, data, err := s.main.RelayModelMethod(r.Context(), r.Method, r.URL.Path, r.URL.Query(), nil, s.relayBillingIdentity(principal.ProxyMainUserID), requestID)
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
		code := "MODEL_NOT_ALLOWED"
		s.writeError(w, http.StatusBadRequest, requestID, code, err.Error())
		return
	}
	if principal.User.Status != "active" {
		s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_DISABLED", "agent user is disabled")
		return
	}
	// Never silently substitute a random ID for a supplied retry identifier:
	// both the legacy lookup and namespaced ID must identify the same request.
	for _, header := range []string{"Idempotency-Key", "X-Request-ID"} {
		if len(strings.TrimSpace(r.Header.Get(header))) > 128 {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST_IDENTIFIER", header+" must not exceed 128 bytes")
			return
		}
	}
	chargeID := requestIDFrom(r, nil)
	legacyChargeID := chargeID
	billingUserID := strings.TrimSpace(principal.ProxyMainUserID)
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(r.Header.Get("X-Request-ID"))
	}
	if key != "" && billingMode == "user_upstream" {
		identity, _ := json.Marshal([]string{agentID, billingUserID, key})
		digest := sha256.Sum256(identity)
		chargeID = "agent-" + hex.EncodeToString(digest[:])
	}
	if billingUserID == "" || strings.TrimSpace(s.cfg.AppCredential) == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "BILLING_NOT_READY", "main user billing identity is not configured")
		return
	}
	// Balance-sensitive work is serialized per real Sub2API user through a
	// durable lease shared by every AgentAPI replica.
	leaseCtx, releaseLease, leaseErr := s.acquireUserOperationLease(r.Context(), billingUserID)
	if leaseErr != nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "SETTLEMENT_LOCK_UNAVAILABLE", "billing operation could not acquire its shared lock")
		return
	}
	defer releaseLease()
	r = r.WithContext(leaseCtx)
	currentUser, userErr := s.store.User(agentID, principal.ProxyMainUserID)
	if userErr != nil {
		if errors.Is(userErr, errNotFound) {
			s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_NOT_FOUND", "Agent user mapping is unavailable")
			return
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to verify mapped user status")
		return
	}
	if currentUser.Status != "active" {
		s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_DISABLED", "agent user is disabled")
		return
	}
	principal.User = currentUser
	// Before namespaced IDs were introduced, the raw client key was stored.
	// Preserve that user's replay barrier without renaming records referenced
	// by task mappings or by the main site's usage logs.
	if chargeID != legacyChargeID {
		legacy, lookupErr := s.store.Settlement(agentID, legacyChargeID)
		if lookupErr == nil && legacy.ProxyMainUserID == billingUserID {
			s.writeSettlementReplay(w, requestID, legacy)
			return
		}
		if lookupErr != nil && !errors.Is(lookupErr, errNotFound) {
			s.writeError(w, http.StatusInternalServerError, requestID, "SETTLEMENT_LOOKUP_FAILED", "failed to load legacy request settlement")
			return
		}
	}
	// The settlement row is the durable request idempotency record. A retry
	// must never send the same model operation to the main site a second time,
	// even if the first process died after the upstream call.
	if existing, lookupErr := s.store.Settlement(agentID, chargeID); lookupErr == nil {
		if existing.ProxyMainUserID != billingUserID {
			s.writeError(w, 409, requestID, "IDEMPOTENCY_CONFLICT", "request identifier belongs to another user")
			return
		}
		s.writeSettlementReplay(w, requestID, existing)
		return
	} else if !errors.Is(lookupErr, errNotFound) {
		s.writeError(w, http.StatusInternalServerError, requestID, "SETTLEMENT_LOOKUP_FAILED", "failed to load request settlement")
		return
	}
	model := requestModelName(r.Header.Get("Content-Type"), body)
	mainBalance, balanceErr := s.main.AdminGetUser(r.Context(), billingUserID)
	if balanceErr != nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "BILLING_BALANCE_UNAVAILABLE", "main user balance could not be read")
		return
	}
	if !mainBalance.BalancePositive {
		s.writeError(w, http.StatusPaymentRequired, requestID, "MAIN_USER_BALANCE_INSUFFICIENT", "main user balance is insufficient")
		return
	}
	before := mainBalance.Balance
	settlement, created, err := s.store.PrepareDirectSettlement(agentID, billingUserID, billingUserID, chargeID, chargeID, s.cfg.MaxRequestCostCents)
	if err != nil {
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
	_ = s.store.SetSettlementModel(agentID, chargeID, model)
	if err := s.store.SetSettlementAPIKey(agentID, chargeID, principal.APIKeyID); err != nil {
		_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "local API key attribution could not be persisted")
		s.writeError(w, http.StatusServiceUnavailable, requestID, "API_KEY_ATTRIBUTION_FAILED", "request was not forwarded because API key attribution could not be saved")
		return
	}

	if modelRequestWantsStream(r.Header.Get("Accept"), r.Header.Get("Content-Type"), body) {
		s.relayStreamingModel(w, r, requestID, agentID, billingMode, billingUserID, chargeID, before, body, r.Header.Get("Content-Type"))
		return
	}

	status, headers, responseBody, relayErr := s.main.RelayModelMethodWithContentType(r.Context(), r.Method, r.URL.Path, r.URL.Query(), body, r.Header.Get("Content-Type"), s.relayBillingIdentity(principal.ProxyMainUserID), chargeID)
	if relayErr != nil {
		// A transport failure does not tell us whether Sub2API received and
		// charged the request. Keep the reservation and reconcile it later.
		if err := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, relayErr.Error()); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "request outcome is uncertain and local settlement could not be recorded")
			return
		}
		s.writeMainError(w, requestID, relayErr)
		return
	}
	if status < 200 || status >= 300 {
		if status == http.StatusTooManyRequests || status >= 500 {
			if err := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, fmt.Sprintf("upstream status %d; charge is uncertain", status)); err != nil {
				s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "request outcome is uncertain and local settlement could not be recorded")
				return
			}
			copyResponse(w, status, headers, responseBody)
			return
		}
		if err := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "released", 0, fmt.Sprintf("upstream status %d", status)); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream rejection was received but local reservation could not be released")
			return
		}
		copyResponse(w, status, headers, responseBody)
		return
	}
	if r.URL.Path == "/v1/videos" {
		taskID, taskStatus := videoTaskIdentity(responseBody)
		if taskID == "" {
			_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "video creation response did not contain a task id")
			s.writeJSON(w, http.StatusBadGateway, apiResponse{Code: http.StatusBadGateway, Message: "video task was created but its id could not be tracked", Reason: "VIDEO_TASK_ID_MISSING", RequestID: requestID})
			return
		}
		if _, err := s.store.RecordVideoTask(agentID, principal.ProxyMainUserID, taskID, chargeID, taskStatus); err != nil {
			_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "video task mapping could not be persisted")
			s.writeJSON(w, http.StatusServiceUnavailable, apiResponse{Code: http.StatusServiceUnavailable, Message: "video task was created but local tracking failed", Reason: "VIDEO_TASK_TRACKING_FAILED", RequestID: requestID, Data: map[string]string{"task_id": taskID}})
			return
		}
	}
	if r.URL.Path == "/v1/images/generations/async" || r.URL.Path == "/v1/images/edits/async" {
		taskID, taskStatus := imageTaskIdentity(responseBody)
		if taskID == "" {
			_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "async image response did not contain a task id")
			s.writeJSON(w, http.StatusBadGateway, apiResponse{Code: http.StatusBadGateway, Message: "image task was accepted but its id could not be tracked", Reason: "IMAGE_TASK_ID_MISSING", RequestID: requestID})
			return
		}
		if _, err := s.store.RecordImageTask(agentID, principal.ProxyMainUserID, taskID, chargeID, taskStatus); err != nil {
			_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "image task mapping could not be persisted")
			s.writeJSON(w, http.StatusServiceUnavailable, apiResponse{Code: http.StatusServiceUnavailable, Message: "image task was accepted but local tracking failed", Reason: "IMAGE_TASK_TRACKING_FAILED", RequestID: requestID, Data: map[string]string{"task_id": taskID}})
			return
		}
	}
	// The main usage log is the authoritative source for a request charge. A
	// balance delta is only a compatibility fallback for older Sub2API builds
	// that do not expose the per-Agent runtime usage endpoint; it must never override a usage row.
	if usage, available, usageErr := s.mainUsageForRequest(r.Context(), billingUserID, chargeID); available {
		if usageErr != nil {
			_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, usageErr.Error())
			copyResponse(w, status, headers, responseBody)
			return
		}
		if usage == nil {
			_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "main-site usage is not visible yet")
			copyResponse(w, status, headers, responseBody)
			return
		}
		actual := usage.ActualCents
		if billingMode != "user_upstream" && actual > s.cfg.MaxRequestCostCents {
			_ = s.store.FinalizeSettlement(agentID, chargeID, usage.ID, "pending", actual, "authoritative usage exceeds local reservation", usage.Snapshot)
			copyResponse(w, status, headers, responseBody)
			return
		}
		if err := s.store.FinalizeSettlement(agentID, chargeID, usage.ID, "confirmed", actual, "", usage.Snapshot); err != nil {
			slog.Error("main request succeeded; local usage sync failed", "request_id", chargeID, "error", err)
			copyResponse(w, status, headers, responseBody)
			return
		}
		copyResponse(w, status, headers, responseBody)
		return
	}

	if billingMode == "user_upstream" {
		_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "awaiting authoritative main-site usage; balance differences are not billing records")
		copyResponse(w, status, headers, responseBody)
		return
	}
	after, hasAfter := s.readMainBalance(r.Context(), billingUserID)
	if !hasAfter {
		if err := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "user balance could not be read after relay"); err != nil {
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
		if err := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "owner usage is not visible yet"); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream succeeded but local settlement could not be recorded")
			return
		}
		copyResponse(w, status, headers, responseBody)
		return
	}
	if delta > s.cfg.MaxRequestCostCents {
		if err := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", delta, "actual owner charge exceeds local reservation"); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream charge exceeds reservation and local settlement could not be recorded")
			return
		}
		copyResponse(w, status, headers, responseBody)
		return
	}
	if err := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "confirmed", delta, "", MainUsageSnapshot{Source: "user_balance_delta_fallback"}); err != nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "upstream succeeded but local settlement could not be finalized")
		return
	}
	copyResponse(w, status, headers, responseBody)
}

// mainUsageForRequest returns (usage, available, err). available is false
// only when the connected main site predates the per-Agent runtime usage endpoint (404),
// allowing the legacy compatibility path to run. Any other admin API error is
// treated as an uncertain charge and kept pending.
func (s *Server) mainUsageForRequest(ctx context.Context, mainUserID, requestID string) (*MainUsageResult, bool, error) {
	if !s.cfg.MainUsageAPI {
		return nil, false, nil
	}
	items, err := s.main.AdminFindUsageForUser(ctx, mainUserID, requestID)
	if err != nil {
		var apiErr *MainAPIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return nil, false, nil
		}
		return nil, true, err
	}
	for i := range items {
		if items[i].RequestID == requestID {
			return &items[i], true, nil
		}
	}
	return nil, true, nil
}

// relayStreamingModel forwards an explicitly streaming request while keeping
// the local reservation until EOF. If the client disconnects or the upstream
// transport fails, the reservation remains pending for usage reconciliation;
// it is never guessed or silently refunded after partial output.
func (s *Server) handleImageTaskPoll(w http.ResponseWriter, r *http.Request, requestID string, principal modelPrincipal) {
	agentID := principal.Tenant.AgentID
	taskID := strings.TrimPrefix(r.URL.Path, "/v1/images/tasks/")
	task, err := s.store.ImageTask(agentID, taskID)
	if err != nil || task.MainUserID != principal.ProxyMainUserID {
		s.writeError(w, http.StatusNotFound, requestID, "IMAGE_TASK_NOT_FOUND", "image task was not found for this agent user")
		return
	}
	billingUserID := strings.TrimSpace(task.MainUserID)
	if settlement, settlementErr := s.store.Settlement(agentID, task.RequestID); settlementErr == nil && strings.TrimSpace(settlement.BillingMainUserID) != "" {
		billingUserID = strings.TrimSpace(settlement.BillingMainUserID)
	}
	if billingUserID == "" || strings.TrimSpace(s.cfg.AppCredential) == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "BILLING_NOT_READY", "main user billing identity is not configured")
		return
	}
	leaseCtx, releaseLease, leaseErr := s.acquireUserOperationLease(r.Context(), billingUserID)
	if leaseErr != nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "SETTLEMENT_LOCK_UNAVAILABLE", "image task reconciliation could not acquire its shared lock")
		return
	}
	defer releaseLease()
	r = r.WithContext(leaseCtx)
	status, headers, data, relayErr := s.main.RelayModelMethod(r.Context(), http.MethodGet, r.URL.Path, r.URL.Query(), nil, s.relayBillingIdentity(task.MainUserID), task.RequestID)
	if relayErr != nil {
		s.writeMainError(w, requestID, relayErr)
		return
	}
	if status >= 200 && status < 300 {
		_, imageStatus := imageTaskIdentity(data)
		if imageStatus != "" {
			_ = s.store.UpdateImageTaskStatus(agentID, taskID, imageStatus)
		}
		if settlement, lookupErr := s.store.Settlement(agentID, task.RequestID); lookupErr == nil && settlement.Status == "pending" {
			usage, available, usageErr := s.mainUsageForRequest(r.Context(), billingUserID, task.RequestID)
			switch {
			case !available:
				_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "async image settlement requires authoritative main-site usage; legacy usage API is unavailable")
			case usageErr != nil:
				_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, usageErr.Error())
			case usage == nil && imageTaskFailed(imageStatus):
				_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "image task failed; awaiting authoritative main-site usage: "+imageStatus)
			case usage == nil:
				_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "main-site usage is not visible yet")
			default:
				actual := usage.ActualCents
				if settlement.HasLocalReservation && actual > settlement.ReservedCents {
					_ = s.store.FinalizeSettlement(agentID, task.RequestID, usage.ID, "pending", actual, "authoritative usage exceeds local reservation", usage.Snapshot)
				} else {
					_ = s.store.FinalizeSettlement(agentID, task.RequestID, usage.ID, "confirmed", actual, "", usage.Snapshot)
				}
			}
		}
	}
	copyResponse(w, status, headers, data)
}

func (s *Server) handleVideoPoll(w http.ResponseWriter, r *http.Request, requestID string, principal modelPrincipal) {
	agentID := principal.Tenant.AgentID
	taskID := strings.TrimPrefix(r.URL.Path, "/v1/videos/")
	task, err := s.store.VideoTask(agentID, taskID)
	if err != nil || task.MainUserID != principal.ProxyMainUserID {
		s.writeError(w, http.StatusNotFound, requestID, "VIDEO_TASK_NOT_FOUND", "video task was not found for this agent user")
		return
	}
	billingUserID := strings.TrimSpace(task.MainUserID)
	if settlement, settlementErr := s.store.Settlement(agentID, task.RequestID); settlementErr == nil && strings.TrimSpace(settlement.BillingMainUserID) != "" {
		billingUserID = strings.TrimSpace(settlement.BillingMainUserID)
	}
	if billingUserID == "" || strings.TrimSpace(s.cfg.AppCredential) == "" {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "BILLING_NOT_READY", "main user billing identity is not configured")
		return
	}
	leaseCtx, releaseLease, leaseErr := s.acquireUserOperationLease(r.Context(), billingUserID)
	if leaseErr != nil {
		s.writeError(w, http.StatusServiceUnavailable, requestID, "SETTLEMENT_LOCK_UNAVAILABLE", "video task reconciliation could not acquire its shared lock")
		return
	}
	defer releaseLease()
	r = r.WithContext(leaseCtx)
	status, headers, data, relayErr := s.main.RelayModelMethod(r.Context(), http.MethodGet, r.URL.Path, r.URL.Query(), nil, s.relayBillingIdentity(task.MainUserID), task.RequestID)
	if relayErr != nil {
		s.writeMainError(w, requestID, relayErr)
		return
	}
	if status >= 200 && status < 300 {
		_, upstreamStatus := videoTaskIdentity(data)
		if upstreamStatus != "" {
			_ = s.store.UpdateVideoTaskStatus(agentID, taskID, upstreamStatus)
		}
		if settlement, lookupErr := s.store.Settlement(agentID, task.RequestID); lookupErr == nil && settlement.Status == "pending" {
			usage, available, usageErr := s.mainUsageForRequest(r.Context(), billingUserID, task.RequestID)
			switch {
			case !available:
				_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "video task settlement requires authoritative main-site usage; legacy usage API is unavailable")
			case usageErr != nil:
				_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, usageErr.Error())
			case usage == nil && videoTaskFailed(upstreamStatus):
				_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "video task failed; awaiting authoritative main-site usage: "+upstreamStatus)
			case usage == nil:
				_ = s.store.FinalizeSettlement(agentID, task.RequestID, task.RequestID, "pending", 0, "main-site usage is not visible yet")
			default:
				actual := usage.ActualCents
				if settlement.HasLocalReservation && actual > settlement.ReservedCents {
					_ = s.store.FinalizeSettlement(agentID, task.RequestID, usage.ID, "pending", actual, "authoritative usage exceeds local reservation", usage.Snapshot)
				} else {
					_ = s.store.FinalizeSettlement(agentID, task.RequestID, usage.ID, "confirmed", actual, "", usage.Snapshot)
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
		id := firstNonEmpty(stringValue(value["id"]), stringValue(value["request_id"]), stringValue(value["task_id"]), stringValue(value["video_id"]))
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

func imageTaskIdentity(data []byte) (string, string) {
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return "", ""
	}
	read := func(value map[string]any) (string, string) {
		id := firstNonEmpty(stringValue(value["task_id"]), stringValue(value["id"]))
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
		status = "processing"
	}
	return id, status
}

func imageTaskFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "cancelled", "canceled", "rejected", "expired":
		return true
	default:
		return false
	}
}

func videoTaskFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "cancelled", "canceled", "rejected", "expired":
		return true
	default:
		return false
	}
}

func (s *Server) relayStreamingModel(w http.ResponseWriter, r *http.Request, requestID, agentID, billingMode, billingUserID, chargeID string, before int64, body []byte, contentType string) {
	resp, err := s.main.OpenModelResponse(r.Context(), r.Method, r.URL.Path, r.URL.Query(), body, contentType, s.relayBillingIdentity(billingUserID), chargeID)
	if err != nil {
		if finalizeErr := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, err.Error()); finalizeErr != nil {
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
			_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, readErr.Error())
			s.writeError(w, http.StatusServiceUnavailable, requestID, "UPSTREAM_RESPONSE_UNREADABLE", "upstream response could not be read")
			return
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if finalizeErr := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, fmt.Sprintf("upstream status %d; charge is uncertain", resp.StatusCode)); finalizeErr != nil {
				s.writeError(w, http.StatusServiceUnavailable, requestID, "LOCAL_SETTLEMENT_FAILED", "request outcome is uncertain and local settlement could not be recorded")
				return
			}
			copyResponse(w, resp.StatusCode, filteredResponseHeaders(resp.Header), data)
			return
		}
		if finalizeErr := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "released", 0, fmt.Sprintf("upstream status %d", resp.StatusCode)); finalizeErr != nil {
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
		if finalizeErr := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "stream interrupted: "+copyErr.Error()); finalizeErr != nil {
			slog.Error("failed to persist interrupted streaming settlement", "request_id", requestID, "error", finalizeErr)
		}
		return
	}

	if usage, available, usageErr := s.mainUsageForRequest(r.Context(), billingUserID, chargeID); available {
		if usageErr != nil {
			_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, usageErr.Error())
			return
		}
		if usage == nil {
			_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "main-site usage is not visible yet")
			return
		}
		actual := usage.ActualCents
		if billingMode != "user_upstream" && actual > s.cfg.MaxRequestCostCents {
			_ = s.store.FinalizeSettlement(agentID, chargeID, usage.ID, "pending", actual, "authoritative usage exceeds local reservation", usage.Snapshot)
			return
		}
		if finalizeErr := s.store.FinalizeSettlement(agentID, chargeID, usage.ID, "confirmed", actual, "", usage.Snapshot); finalizeErr != nil {
			slog.Error("failed to finalize streaming usage settlement", "request_id", requestID, "error", finalizeErr)
		}
		return
	}
	if billingMode == "user_upstream" {
		_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "awaiting authoritative main-site usage; balance differences are not billing records")
		return
	}
	after, hasAfter := s.readMainBalance(r.Context(), billingUserID)
	if !hasAfter {
		_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "user balance could not be read after streaming relay")
		return
	}
	delta := before - after
	if delta < 0 {
		delta = 0
	}
	if delta == 0 {
		_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", 0, "owner usage is not visible yet")
		return
	}
	if delta > s.cfg.MaxRequestCostCents {
		_ = s.store.FinalizeSettlement(agentID, chargeID, chargeID, "pending", delta, "actual owner charge exceeds local reservation")
		return
	}
	if finalizeErr := s.store.FinalizeSettlement(agentID, chargeID, chargeID, "confirmed", delta, "", MainUsageSnapshot{Source: "user_balance_delta_fallback"}); finalizeErr != nil {
		slog.Error("failed to finalize streaming settlement", "request_id", requestID, "error", finalizeErr)
		return
	}
}

type settlementReconcileResult struct {
	RequestID   string `json:"request_id"`
	Status      string `json:"status"`
	ActualCents int64  `json:"actual_cents"`
	UsageID     string `json:"usage_id,omitempty"`
	Message     string `json:"message,omitempty"`
}

func (s *Server) reconcileTenantSettlements(ctx context.Context, agentID, requestID string) ([]settlementReconcileResult, error) {
	var records []SettlementRecord
	var err error
	if requestID != "" {
		record, lookupErr := s.store.Settlement(agentID, requestID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		records = []SettlementRecord{record}
	} else {
		records, err = s.store.PendingSettlements(agentID, s.cfg.SettlementReconcileBatch)
		if err != nil {
			return nil, err
		}
	}
	result := make([]settlementReconcileResult, 0, len(records))
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := s.store.MarkReconciliationAttempt(agentID, record.RequestID); err != nil {
			return result, err
		}
		billingUserID := strings.TrimSpace(record.BillingMainUserID)
		if billingUserID == "" {
			result = append(result, settlementReconcileResult{RequestID: record.RequestID, Status: record.Status, Message: "billing user identity is missing; manual review required"})
			continue
		}
		leaseErr := func() error {
			leaseCtx, releaseLease, err := s.acquireUserOperationLease(ctx, billingUserID)
			if err != nil {
				return err
			}
			defer releaseLease()

			// The worker may have waited for another replica. Reload the record so
			// a stale pending snapshot cannot overwrite a terminal settlement.
			current, err := s.store.Settlement(agentID, record.RequestID)
			if err != nil {
				return err
			}
			item := settlementReconcileResult{RequestID: current.RequestID, Status: current.Status, ActualCents: current.ActualCents, UsageID: current.UsageID}
			if current.Status == "released" || current.Status == "reversed" || current.Status == "confirmed" {
				item.Message = "settlement is already finalized"
				result = append(result, item)
				return nil
			}
			usageItems, findErr := s.main.AdminFindUsageForUser(leaseCtx, billingUserID, current.RequestID)
			if findErr != nil {
				if requestID != "" {
					return findErr
				}
				item.Message = "main-site usage lookup failed; kept pending for retry"
				result = append(result, item)
				return nil
			}
			var usage *MainUsageResult
			for i := range usageItems {
				if usageItems[i].RequestID == current.RequestID {
					usage = &usageItems[i]
					break
				}
			}
			if usage == nil {
				item.Status = "pending"
				item.Message = "main-site usage is not visible yet; no local refund was made"
				result = append(result, item)
				return nil
			}
			actual := usage.ActualCents
			item.ActualCents = actual
			item.UsageID = usage.ID
			if current.HasLocalReservation && actual > current.ReservedCents {
				if err := s.store.FinalizeSettlement(agentID, current.RequestID, usage.ID, "pending", actual, "authoritative usage exceeds local reservation", usage.Snapshot); err != nil {
					return err
				}
				item.Status = "pending"
				item.Message = "authoritative charge exceeds local reservation; manual review required"
				result = append(result, item)
				return nil
			}
			if err := s.store.FinalizeSettlement(agentID, current.RequestID, usage.ID, "confirmed", actual, "", usage.Snapshot); err != nil && !errors.Is(err, errSettlementStateConflict) {
				return err
			}
			item.Status = "confirmed"
			item.Message = "settlement confirmed from main-site usage"
			result = append(result, item)
			return nil
		}()
		if leaseErr != nil {
			if requestID != "" {
				return result, leaseErr
			}
			result = append(result, settlementReconcileResult{RequestID: record.RequestID, Status: record.Status, Message: "shared settlement lock unavailable; kept pending for retry"})
		}
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
	Tenant          TenantContext
	ProxyMainUserID string
	User            AgentUserView
	APIKeyID        int64
	APIKeyName      string
	APIKeyPrefix    string
}

// requireModelPrincipal accepts either the browser session or an AgentAPI
// gateway key. The latter is intentionally resolved only against the local
// agent_api_keys table, so a Sub2API JWT, administrator key, or arbitrary
// bearer token cannot be used as a model credential here.
func (s *Server) requireModelPrincipal(w http.ResponseWriter, r *http.Request, requestID string) (modelPrincipal, bool) {
	if tenant, session, user, ok := s.loadTenantSession(r); ok {
		// Cookie-authenticated model requests are state-changing from the
		// browser's perspective. Require the page origin to match the AgentAPI
		// host; programmatic clients should use a local sk-* key instead.
		if r.Method != http.MethodGet && !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin model requests are not allowed")
			return modelPrincipal{}, false
		}
		return modelPrincipal{Tenant: tenant, ProxyMainUserID: session.MainUserID, User: user}, true
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
	resolved, err := s.store.ResolveAPIKeyDetailsAnyTenant(strings.TrimSpace(parts[1]))
	if err != nil {
		if errors.Is(err, errNotFound) {
			s.writeError(w, http.StatusUnauthorized, requestID, "INVALID_AGENT_API_KEY", "AgentAPI API key is invalid or revoked")
			return modelPrincipal{}, false
		}
		s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to resolve AgentAPI API key")
		return modelPrincipal{}, false
	}
	user, err := s.store.User(resolved.AgentID, resolved.MainUserID)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, requestID, "AGENT_USER_NOT_FOUND", "AgentAPI user mapping is unavailable")
		return modelPrincipal{}, false
	}
	if user.Status != "active" {
		s.writeError(w, http.StatusForbidden, requestID, "AGENT_USER_DISABLED", "agent user is disabled")
		return modelPrincipal{}, false
	}
	tenant := TenantContext{AgentID: resolved.AgentID, MainUserID: resolved.MainUserID, Role: tenantRoleMember, Source: tenantSourceAPIKey}
	return modelPrincipal{Tenant: tenant, ProxyMainUserID: resolved.MainUserID, User: user, APIKeyID: resolved.ID, APIKeyName: resolved.Name, APIKeyPrefix: resolved.Prefix}, true
}

func (s *Server) readMainBalance(ctx context.Context, mainUserID string) (int64, bool) {
	user, err := s.main.AdminGetUser(ctx, mainUserID)
	if err != nil {
		return 0, false
	}
	return user.Balance, true
}

func (s *Server) relayBillingIdentity(proxyMainUserID string) string {
	return strings.TrimSpace(proxyMainUserID)
}

func (s *Server) tenantOwnerMainUserID(agent AgentView) string {
	return strings.TrimSpace(agent.OwnerMainUserID)
}

func (s *Server) syncOwnerBalanceForTenant(ctx context.Context, agentID, requestID string) (AgentView, error) {
	agent, err := s.store.Agent(agentID)
	if err != nil {
		return AgentView{}, err
	}
	ownerID := s.tenantOwnerMainUserID(agent)
	if ownerID == "" {
		return AgentView{}, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_OWNER_MISSING", Message: "billing owner is not configured"}
	}
	leaseCtx, releaseLease, err := s.acquireUserOperationLease(ctx, ownerID)
	if err != nil {
		return AgentView{}, err
	}
	defer releaseLease()
	user, err := s.main.AdminGetUser(leaseCtx, ownerID)
	if err != nil {
		return AgentView{}, err
	}
	return s.store.SyncOwnerBalance(agent.ID, ownerID, user.Balance, requestID)
}

// syncAndAllocateRechargeOrderForTenant holds the same shared per-owner lease
// used by model settlement for the entire sync+allocation sequence. Without
// this critical section a model request could spend the owner balance after
// the snapshot but before a paid recharge consumes the local available amount.
func (s *Server) syncAndAllocateRechargeOrderForTenant(ctx context.Context, agentID, requestID, orderNo string) (RechargeOrder, error) {
	agent, err := s.store.Agent(agentID)
	if err != nil {
		return RechargeOrder{}, err
	}
	ownerID := s.tenantOwnerMainUserID(agent)
	if ownerID == "" {
		return RechargeOrder{}, &MainAPIError{Status: http.StatusServiceUnavailable, Code: "MAIN_OWNER_MISSING", Message: "billing owner is not configured"}
	}
	leaseCtx, releaseLease, err := s.acquireUserOperationLease(ctx, ownerID)
	if err != nil {
		return RechargeOrder{}, err
	}
	defer releaseLease()
	user, err := s.main.AdminGetUser(leaseCtx, ownerID)
	if err != nil {
		return RechargeOrder{}, err
	}
	if _, err := s.store.SyncOwnerBalance(agent.ID, ownerID, user.Balance, requestID); err != nil {
		return RechargeOrder{}, err
	}
	return s.store.AllocateRechargeOrder(agent.ID, orderNo)
}

const userOperationLeaseDuration = 30 * time.Second

// acquireUserOperationLease serializes balance-sensitive work for one real
// Sub2API user across every AgentAPI process sharing the database. A short
// renewable lease replaces the old process-local mutex: crashed replicas stop
// renewing and cannot block the user forever, while a lost lease cancels the
// in-flight upstream context before this process can continue unsafely.
func (s *Server) acquireUserOperationLease(parent context.Context, mainUserID string) (context.Context, func(), error) {
	mainUserID = strings.TrimSpace(mainUserID)
	if mainUserID == "" {
		return nil, nil, fmt.Errorf("main user id is required for operation lease")
	}
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, nil, fmt.Errorf("create operation lease token: %w", err)
	}
	lockKey := "settlement:user:" + mainUserID
	ownerToken := hex.EncodeToString(tokenBytes)
	leaseCtx, cancelLease := context.WithCancel(parent)
	if err := s.store.AcquireOperationLease(leaseCtx, lockKey, ownerToken, userOperationLeaseDuration); err != nil {
		cancelLease()
		return nil, nil, err
	}

	done := make(chan struct{})
	var releaseOnce sync.Once
	go func() {
		ticker := time.NewTicker(userOperationLeaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-leaseCtx.Done():
				return
			case <-ticker.C:
				renewCtx, cancelRenew := context.WithTimeout(context.Background(), 5*time.Second)
				owned, err := s.store.RenewOperationLease(renewCtx, lockKey, ownerToken, userOperationLeaseDuration)
				cancelRenew()
				if err != nil || !owned {
					slog.Error("shared user operation lease lost", "main_user_id", mainUserID, "error", err)
					cancelLease()
					return
				}
			}
		}
	}()

	release := func() {
		releaseOnce.Do(func() {
			close(done)
			releaseCtx, cancelRelease := context.WithTimeout(context.Background(), 5*time.Second)
			if err := s.store.ReleaseOperationLease(releaseCtx, lockKey, ownerToken); err != nil {
				slog.Error("release shared user operation lease failed", "main_user_id", mainUserID, "error", err)
			}
			cancelRelease()
			cancelLease()
		})
	}
	return leaseCtx, release, nil
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request, requestID string) (Session, AgentUserView, bool) {
	_, session, user, ok := s.requireTenantSession(w, r, requestID)
	return session, user, ok
}

func (s *Server) requireTenantSession(w http.ResponseWriter, r *http.Request, requestID string) (TenantContext, Session, AgentUserView, bool) {
	tenant, session, user, ok := s.loadTenantSession(r)
	if !ok {
		s.writeError(w, http.StatusUnauthorized, requestID, "UNAUTHORIZED", "AgentAPI session is required")
		return TenantContext{}, Session{}, AgentUserView{}, false
	}
	return tenant, session, user, true
}

func (s *Server) loadSession(r *http.Request) (Session, AgentUserView, bool) {
	_, session, user, ok := s.loadTenantSession(r)
	return session, user, ok
}

func (s *Server) loadTenantSession(r *http.Request) (TenantContext, Session, AgentUserView, bool) {
	cookie, err := r.Cookie(s.cfg.CookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return TenantContext{}, Session{}, AgentUserView{}, false
	}
	session, err := s.store.LoadSession(cookie.Value)
	if err != nil {
		return TenantContext{}, Session{}, AgentUserView{}, false
	}
	tenant, ok := s.tenantContextFromSession(session)
	if !ok {
		return TenantContext{}, Session{}, AgentUserView{}, false
	}
	user, err := s.store.User(tenant.AgentID, session.MainUserID)
	if err != nil || user.Status != "active" {
		return TenantContext{}, Session{}, AgentUserView{}, false
	}
	return tenant, session, user, true
}

func (s *Server) isAgentAdmin(session Session) bool {
	// Agent administration is an explicit per-instance ownership grant. A
	// user's global role on Sub2API must not silently grant control of every
	// AgentAPI wallet reachable through this instance.
	tenant, ok := s.tenantContextFromSession(session)
	return ok && tenant.Role == tenantRoleOwner
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

func (s *Server) browserAuthResponseForTenant(raw []byte, mainUserID string, agent AgentView) map[string]any {
	return map[string]any{
		"access_token":  "",
		"refresh_token": "",
		"expires_in":    int(sessionTTL.Seconds()),
		"token_type":    "Cookie",
		"user":          s.browserUserForTenant(raw, mainUserID, agent),
	}
}

func (s *Server) browserUserForTenant(raw []byte, mainUserID string, agent AgentView) map[string]any {
	// Only the public profile fields consumed by AgentAPI may cross this
	// boundary. New main-site fields must not become browser-visible by default.
	var upstream map[string]json.RawMessage
	_ = json.Unmarshal(raw, &upstream)
	result := make(map[string]any)
	for _, field := range []string{"email", "username", "display_name", "avatar_url", "status"} {
		var value string
		if data, ok := upstream[field]; ok && string(data) != "null" && json.Unmarshal(data, &value) == nil {
			result[field] = value
		}
	}
	if numericID, err := strconv.ParseInt(mainUserID, 10, 64); err == nil && numericID > 0 {
		result["id"] = numericID
	} else {
		result["id"] = mainUserID
	}
	// AgentAPI's copied admin UI must not become a main-site admin console. The
	// separate agent_admin flag is consumed by the AgentAPI view only.
	result["role"] = "user"
	result["agent_admin"] = strings.TrimSpace(agent.OwnerMainUserID) == strings.TrimSpace(mainUserID)
	result["agent_id"] = agent.ID
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

func (s *Server) recordTenantAudit(agentID, actorType, actorID, operation, targetType, targetID, requestID, result, reason string) {
	if err := s.store.RecordAuditEvent(AuditEvent{
		ActorType: actorType, ActorID: actorID, AgentID: agentID,
		Operation: operation, TargetType: targetType, TargetID: targetID,
		RequestID: requestID, Result: result, Reason: reason,
	}); err != nil {
		slog.Error("failed to persist audit event", "agent_id", agentID, "operation", operation, "request_id", requestID, "error", err)
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

func isAllowedModelPath(rawPath, method string) bool {
	if method != http.MethodGet && method != http.MethodPost {
		return false
	}
	if rawPath == "/v1/images/generations/async" || rawPath == "/v1/images/edits/async" {
		return method == http.MethodPost
	}
	switch rawPath {
	case "/v1/models":
		return method == http.MethodGet
	case "/v1/chat/completions", "/v1/responses", "/v1/images/generations", "/v1/images/edits", "/v1/videos":
		return method == http.MethodPost
	default:
		videoID := strings.TrimPrefix(rawPath, "/v1/videos/")
		if method == http.MethodGet && strings.HasPrefix(rawPath, "/v1/videos/") && videoID != "" && videoID != "." && videoID != ".." && !strings.Contains(videoID, "/") {
			return true
		}
		imageTaskID := strings.TrimPrefix(rawPath, "/v1/images/tasks/")
		return method == http.MethodGet && strings.HasPrefix(rawPath, "/v1/images/tasks/") && imageTaskID != "" && imageTaskID != "." && imageTaskID != ".." && !strings.Contains(imageTaskID, "/")
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
			return fmt.Errorf("invalid multipart form content type")
		}
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		modelFieldSeen := false
		for {
			part, nextErr := reader.NextPart()
			if errors.Is(nextErr, io.EOF) {
				return nil
			}
			if nextErr != nil {
				return fmt.Errorf("invalid multipart form body")
			}
			if part.FormName() != "model" {
				continue
			}
			if modelFieldSeen {
				return fmt.Errorf("multipart form must not contain duplicate model fields")
			}
			modelFieldSeen = true
			modelBytes, readErr := io.ReadAll(io.LimitReader(part, 129))
			if readErr != nil {
				return fmt.Errorf("failed to read multipart model field")
			}
			if len(modelBytes) > 128 {
				return fmt.Errorf("multipart model field is too long")
			}
			model := strings.TrimSpace(string(modelBytes))
			if model == "" {
				continue
			}
			if _, ok := publicModelNames[model]; !ok {
				return fmt.Errorf("model %q is not in the AgentAPI public model catalog", model)
			}
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

func requestModelName(contentType string, body []byte) string {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err == nil && mediaType == "multipart/form-data" && params["boundary"] != "" {
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, nextErr := reader.NextPart()
			if errors.Is(nextErr, io.EOF) || nextErr != nil {
				return ""
			}
			if part.FormName() != "model" {
				continue
			}
			value, readErr := io.ReadAll(io.LimitReader(part, 256))
			if readErr != nil {
				return ""
			}
			return strings.TrimSpace(string(value))
		}
	}
	var payload struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.Model)
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
