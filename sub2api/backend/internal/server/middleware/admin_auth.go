// Package middleware provides HTTP middleware for authentication, authorization, and request processing.
package middleware

import (
	"crypto/subtle"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const agentProvisioningWorkerCredentialHeader = "X-AgentAPI-Provisioning-Credential"

// NewAdminAuthMiddleware 创建管理员认证中间件
func NewAdminAuthMiddleware(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
) AdminAuthMiddleware {
	return AdminAuthMiddleware(adminAuth(authService, userService, settingService, auditService, loadAgentProvisioningWorkerCredential()))
}

// adminAuth 管理员认证中间件实现。
// 支持 Admin API Key、管理员 JWT，以及只允许调用开站 worker 路由的专用凭据。
func adminAuth(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
	provisioningWorkerCredential string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// WebSocket upgrade requests cannot set Authorization headers in browsers.
		// For admin WebSocket endpoints (e.g. Ops realtime), allow passing the JWT via
		// Sec-WebSocket-Protocol (subprotocol list) using a prefixed token item:
		//   Sec-WebSocket-Protocol: sub2api-admin, jwt.<token>
		if isWebSocketUpgradeRequest(c) {
			if token := extractJWTFromWebSocketSubprotocol(c); token != "" {
				if !validateJWTForAdmin(c, token, authService, userService, settingService, auditService) {
					return
				}
				c.Next()
				return
			}
		}

		if credential := strings.TrimSpace(c.GetHeader(agentProvisioningWorkerCredentialHeader)); credential != "" {
			if len(provisioningWorkerCredential) < 32 {
				AbortWithError(c, 503, "AGENT_PROVISIONING_WORKER_AUTH_UNAVAILABLE", "Provisioning worker authentication is not configured")
				return
			}
			if subtle.ConstantTimeCompare([]byte(credential), []byte(provisioningWorkerCredential)) != 1 {
				AbortWithError(c, 401, "UNAUTHORIZED", "Invalid provisioning worker credential")
				return
			}
			if !isAgentProvisioningWorkerRequest(c) {
				AbortWithError(c, 403, "FORBIDDEN", "Provisioning worker credential is not allowed for this operation")
				return
			}
			admin, err := userService.GetFirstAdmin(c.Request.Context())
			if err != nil {
				AbortWithError(c, 500, "INTERNAL_ERROR", "No admin user found")
				return
			}
			c.Set(string(ContextKeyUser), AuthSubject{UserID: admin.ID, Concurrency: admin.Concurrency})
			c.Set(string(ContextKeyUserRole), admin.Role)
			c.Set(ContextKeyAuthEmail, admin.Email)
			c.Set("auth_method", "agent_provisioning_worker")
			c.Next()
			return
		}

		// 检查 x-api-key header（Admin API Key 认证）
		apiKey := c.GetHeader("x-api-key")
		if apiKey != "" {
			if !validateAdminAPIKey(c, apiKey, settingService, userService) {
				return
			}
			c.Next()
			return
		}

		// 检查 Authorization header（JWT 认证）
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token := strings.TrimSpace(parts[1])
				if token == "" {
					AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
					return
				}
				if !validateJWTForAdmin(c, token, authService, userService, settingService, auditService) {
					return
				}
				c.Next()
				return
			}
		}

		// 无有效认证信息
		AbortWithError(c, 401, "UNAUTHORIZED", "Authorization required")
	}
}

func loadAgentProvisioningWorkerCredential() string {
	if path := strings.TrimSpace(os.Getenv("AGENT_PROVISIONING_WORKER_CREDENTIAL_FILE")); path != "" {
		if !filepath.IsAbs(path) {
			return ""
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return ""
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}
	return strings.TrimSpace(os.Getenv("AGENT_PROVISIONING_WORKER_CREDENTIAL"))
}

func isAgentProvisioningWorkerRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	const agentsPath = "/api/v1/agent-provisioning/agents"
	path := c.Request.URL.Path
	if c.Request.Method == "GET" && path == agentsPath+"/stream" {
		return c.Request.URL.RawQuery == ""
	}
	if c.Request.Method == "GET" && path == agentsPath {
		query, err := url.ParseQuery(c.Request.URL.RawQuery)
		if err != nil {
			return false
		}
		for key, values := range query {
			if len(values) != 1 {
				return false
			}
			switch key {
			case "status":
				if values[0] != "pending" && values[0] != "provisioning" {
					return false
				}
			case "page", "page_size":
				value, err := strconv.Atoi(values[0])
				if err != nil || value < 1 || (key == "page" && value > 1_000_000) || (key == "page_size" && value > 100) {
					return false
				}
			default:
				return false
			}
		}
		return query.Get("status") == "pending" || query.Get("status") == "provisioning"
	}
	if !strings.HasPrefix(path, agentsPath+"/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, agentsPath+"/"), "/")
	if len(parts) == 1 && isProvisioningAgentID(parts[0]) {
		return c.Request.Method == "GET" && c.Request.URL.RawQuery == ""
	}
	if len(parts) != 2 || !isProvisioningAgentID(parts[0]) || c.Request.URL.RawQuery != "" {
		return false
	}
	return c.Request.Method == "POST" && (parts[1] == "claim" || parts[1] == "lease" || parts[1] == "runtime-credentials") ||
		(c.Request.Method == "PATCH" && parts[1] == "progress") ||
		(c.Request.Method == "POST" && parts[1] == "activate")
}

func isProvisioningAgentID(value string) bool {
	if len(value) != 36 || !strings.HasPrefix(value, "agt_") {
		return false
	}
	for _, char := range value[4:] {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

// MaskedRequestCredential keeps the dedicated provisioning credential out of
// raw audit metadata if this middleware is used for one of its allowed routes.
func maskedAgentProvisioningWorkerCredential(c *gin.Context) string {
	if c == nil {
		return ""
	}
	credential := strings.TrimSpace(c.GetHeader(agentProvisioningWorkerCredentialHeader))
	if credential == "" {
		return ""
	}
	return "agent_provisioning_worker " + service.MaskAuditCredential(credential)
}

func isWebSocketUpgradeRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	// RFC6455 handshake uses:
	//   Connection: Upgrade
	//   Upgrade: websocket
	upgrade := strings.ToLower(strings.TrimSpace(c.GetHeader("Upgrade")))
	if upgrade != "websocket" {
		return false
	}
	connection := strings.ToLower(c.GetHeader("Connection"))
	return strings.Contains(connection, "upgrade")
}

func extractJWTFromWebSocketSubprotocol(c *gin.Context) string {
	if c == nil {
		return ""
	}
	raw := strings.TrimSpace(c.GetHeader("Sec-WebSocket-Protocol"))
	if raw == "" {
		return ""
	}

	// The header is a comma-separated list of tokens. We reserve the prefix "jwt."
	// for carrying the admin JWT.
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(part)
		if strings.HasPrefix(p, "jwt.") {
			token := strings.TrimSpace(strings.TrimPrefix(p, "jwt."))
			if token != "" {
				return token
			}
		}
	}
	return ""
}

// validateAdminAPIKey 验证管理员 API Key
func validateAdminAPIKey(
	c *gin.Context,
	key string,
	settingService *service.SettingService,
	userService *service.UserService,
) bool {
	storedKey, err := settingService.GetAdminAPIKey(c.Request.Context())
	if err != nil {
		AbortWithError(c, 500, "INTERNAL_ERROR", "Internal server error")
		return false
	}

	// 未配置或不匹配，统一返回相同错误（避免信息泄露）
	if storedKey == "" || subtle.ConstantTimeCompare([]byte(key), []byte(storedKey)) != 1 {
		AbortWithError(c, 401, "INVALID_ADMIN_KEY", "Invalid admin API key")
		return false
	}

	// 获取真实的管理员用户
	admin, err := userService.GetFirstAdmin(c.Request.Context())
	if err != nil {
		AbortWithError(c, 500, "INTERNAL_ERROR", "No admin user found")
		return false
	}

	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      admin.ID,
		Concurrency: admin.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), admin.Role)
	c.Set(ContextKeyAuthEmail, admin.Email)
	c.Set("auth_method", "admin_api_key")
	return true
}

// validateJWTForAdmin 验证 JWT 并检查管理员权限
func validateJWTForAdmin(
	c *gin.Context,
	token string,
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
) bool {
	// 验证 JWT token
	claims, err := authService.ValidateToken(token)
	if err != nil {
		if errors.Is(err, service.ErrTokenExpired) {
			AbortWithError(c, 401, "TOKEN_EXPIRED", "Token has expired")
			return false
		}
		AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
		return false
	}

	// 从数据库获取用户
	user, err := userService.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
		} else {
			AbortWithError(c, 500, "INTERNAL_ERROR", "Failed to load user")
		}
		return false
	}

	// 检查用户状态
	if !user.IsActive() {
		AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
		return false
	}

	// 校验 TokenVersion，确保管理员改密后旧 token 失效
	if claims.TokenVersion != user.TokenVersion {
		AbortWithError(c, 401, "TOKEN_REVOKED", "Token has been revoked (password changed)")
		return false
	}

	// 会话绑定校验：IP/UA 任一变化即撤销会话（功能可在系统设置中关闭）
	if !enforceSessionBinding(c, authService, settingService, auditService, claims) {
		return false
	}

	// 检查管理员权限
	if !user.IsAdmin() {
		AbortWithError(c, 403, "FORBIDDEN", "Admin access required")
		return false
	}

	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      user.ID,
		Concurrency: user.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), user.Role)
	c.Set(ContextKeyAuthEmail, user.Email)
	c.Set(ContextKeySessionID, claims.SessionID)
	c.Set("auth_method", "jwt")

	return true
}
