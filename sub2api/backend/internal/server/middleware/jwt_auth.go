package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// BrowserSessionCookieName is an opaque HttpOnly session for browser
// navigations such as satellite SSO. The access JWT is kept server-side.
const BrowserSessionCookieName = "sub2api_session"

// ensureBrowserSessionCookie bridges existing API-token browser sessions to
// top-level navigations (such as satellite SSO), which cannot attach a Bearer
// header. Only the opaque session id is sent to the browser.
func ensureBrowserSessionCookie(c *gin.Context, authService *service.AuthService, accessToken string) {
	if c == nil || authService == nil || strings.TrimSpace(accessToken) == "" {
		return
	}
	if _, err := c.Cookie(BrowserSessionCookieName); err == nil {
		return
	}
	sessionID, ttl, err := authService.IssueBrowserSession(c.Request.Context(), accessToken, service.BrowserSessionTTL)
	if err != nil || strings.TrimSpace(sessionID) == "" || ttl <= 0 {
		return
	}
	secure := c.Request.TLS != nil || strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     BrowserSessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(ttl / time.Second),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// NewJWTAuthMiddleware 创建 JWT 认证中间件
func NewJWTAuthMiddleware(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
) JWTAuthMiddleware {
	return JWTAuthMiddleware(jwtAuth(authService, userService, userService, settingService, auditService))
}

type jwtUserReader interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

type userActivityToucher interface {
	TouchLastActiveForUser(ctx context.Context, user *service.User)
}

// jwtAuth JWT认证中间件实现
func jwtAuth(
	authService *service.AuthService,
	userService jwtUserReader,
	activityToucher userActivityToucher,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Authorization header中提取token；浏览器顶层导航没有机会设置
		// Authorization，因此允许由登录响应设置的 opaque HttpOnly Cookie
		// 作为同等凭据；Cookie 本身不是 JWT。
		authHeader := c.GetHeader("Authorization")
		tokenString := ""
		if authHeader != "" {
			// 验证Bearer scheme
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				AbortWithError(c, 401, "INVALID_AUTH_HEADER", "Authorization header format must be 'Bearer {token}'")
				return
			}
			tokenString = strings.TrimSpace(parts[1])
			if tokenString == "" {
				AbortWithError(c, 401, "EMPTY_TOKEN", "Token cannot be empty")
				return
			}
		} else if cookie, err := c.Cookie(BrowserSessionCookieName); err == nil {
			var resolveErr error
			tokenString, resolveErr = authService.ResolveBrowserSession(c.Request.Context(), cookie)
			if resolveErr != nil {
				tokenString = ""
			}
		}
		if tokenString == "" {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization header is required")
			return
		}

		// 验证token
		claims, err := authService.ValidateToken(tokenString)
		if err != nil {
			if errors.Is(err, service.ErrTokenExpired) {
				AbortWithError(c, 401, "TOKEN_EXPIRED", "Token has expired")
				return
			}
			AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
			return
		}

		// 从数据库获取最新的用户信息
		user, err := userService.GetByID(c.Request.Context(), claims.UserID)
		if err != nil {
			if errors.Is(err, service.ErrUserNotFound) {
				AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
			} else {
				AbortWithError(c, 500, "INTERNAL_ERROR", "Failed to load user")
			}
			return
		}

		// 检查用户状态
		if !user.IsActive() {
			AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
			return
		}

		// Security: Validate TokenVersion to ensure token hasn't been invalidated
		// This check ensures tokens issued before a password change are rejected
		if claims.TokenVersion != user.TokenVersion {
			AbortWithError(c, 401, "TOKEN_REVOKED", "Token has been revoked (password changed)")
			return
		}

		// 会话绑定校验：IP/UA 任一变化即撤销会话（功能可在系统设置中关闭）
		if !enforceSessionBinding(c, authService, settingService, auditService, claims) {
			return
		}
		if authHeader != "" {
			ensureBrowserSessionCookie(c, authService, tokenString)
		}

		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      user.ID,
			Concurrency: user.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), user.Role)
		c.Set(ContextKeyAuthEmail, user.Email)
		c.Set(ContextKeySessionID, claims.SessionID)
		if activityToucher != nil {
			activityToucher.TouchLastActiveForUser(c.Request.Context(), user)
		}

		c.Next()
	}
}

// Deprecated: prefer GetAuthSubjectFromContext in auth_subject.go.
