//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminAuthJWTValidatesTokenVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)

	admin := &service.User{
		ID:           1,
		Email:        "admin@example.com",
		Role:         service.RoleAdmin,
		Status:       service.StatusActive,
		TokenVersion: 2,
		Concurrency:  1,
	}

	userRepo := &stubUserRepo{
		getFirstAdmin: func(context.Context) (*service.User, error) { return admin, nil },
		getByID: func(ctx context.Context, id int64) (*service.User, error) {
			if id != admin.ID {
				return nil, service.ErrUserNotFound
			}
			clone := *admin
			return &clone, nil
		},
	}
	userService := service.NewUserService(userRepo, nil, nil, nil)

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(authService, userService, nil, nil)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	t.Run("token_version_mismatch_rejected", func(t *testing.T) {
		token, err := authService.GenerateToken(context.Background(), &service.User{
			ID:           admin.ID,
			Email:        admin.Email,
			Role:         admin.Role,
			TokenVersion: admin.TokenVersion - 1,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Contains(t, w.Body.String(), "TOKEN_REVOKED")
	})

	t.Run("token_version_match_allows", func(t *testing.T) {
		token, err := authService.GenerateToken(context.Background(), &service.User{
			ID:           admin.ID,
			Email:        admin.Email,
			Role:         admin.Role,
			TokenVersion: admin.TokenVersion,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("websocket_token_version_mismatch_rejected", func(t *testing.T) {
		token, err := authService.GenerateToken(context.Background(), &service.User{
			ID:           admin.ID,
			Email:        admin.Email,
			Role:         admin.Role,
			TokenVersion: admin.TokenVersion - 1,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin, jwt."+token)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Contains(t, w.Body.String(), "TOKEN_REVOKED")
	})

	t.Run("websocket_token_version_match_allows", func(t *testing.T) {
		token, err := authService.GenerateToken(context.Background(), &service.User{
			ID:           admin.ID,
			Email:        admin.Email,
			Role:         admin.Role,
			TokenVersion: admin.TokenVersion,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin, jwt."+token)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestAdminAuthProvisioningWorkerCredentialIsStrictlyScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const credential = "worker-only-provisioning-credential-test-value"
	admin := &service.User{ID: 91, Email: "platform-admin@example.com", Role: service.RoleAdmin, Status: service.StatusActive, Concurrency: 2}
	userService := service.NewUserService(&stubUserRepo{
		getFirstAdmin: func(context.Context) (*service.User, error) { return admin, nil },
	}, nil, nil, nil)
	t.Setenv("AGENT_PROVISIONING_WORKER_CREDENTIAL", credential)
	secretPath := filepath.Join(t.TempDir(), "worker-credential")
	if err := os.WriteFile(secretPath, []byte(credential), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_PROVISIONING_WORKER_CREDENTIAL_FILE", secretPath)

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(nil, userService, nil, nil)))
	router.Any("/*path", func(c *gin.Context) {
		subject, _ := GetAuthSubjectFromContext(c)
		c.JSON(http.StatusOK, gin.H{"auth_method": c.GetString("auth_method"), "user_id": subject.UserID})
	})
	testCases := []struct {
		name   string
		method string
		path   string
		status int
	}{
		{name: "pending list", method: http.MethodGet, path: "/api/v1/agent-provisioning/agents?status=pending&page=1&page_size=100", status: http.StatusOK},
		{name: "provisioning list", method: http.MethodGet, path: "/api/v1/agent-provisioning/agents?status=provisioning", status: http.StatusOK},
		{name: "change stream", method: http.MethodGet, path: "/api/v1/agent-provisioning/agents/stream", status: http.StatusOK},
		{name: "agent status", method: http.MethodGet, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef", status: http.StatusOK},
		{name: "claim", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/claim", status: http.StatusOK},
		{name: "lease renewal", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/lease", status: http.StatusOK},
		{name: "runtime credential registration", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/runtime-credentials", status: http.StatusOK},
		{name: "progress", method: http.MethodPatch, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/progress", status: http.StatusOK},
		{name: "activate", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/activate", status: http.StatusOK},
		{name: "runtime credential revocation", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/runtime-credentials/revoke", status: http.StatusForbidden},
		{name: "claim query rejected", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/claim?force=true", status: http.StatusForbidden},
		{name: "create", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents", status: http.StatusForbidden},
		{name: "suspend", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/suspend", status: http.StatusForbidden},
		{name: "resume", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/resume", status: http.StatusForbidden},
		{name: "revoke", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/revoke", status: http.StatusForbidden},
		{name: "retry", method: http.MethodPost, path: "/api/v1/agent-provisioning/agents/agt_0123456789abcdef0123456789abcdef/retry", status: http.StatusForbidden},
		{name: "failed list", method: http.MethodGet, path: "/api/v1/agent-provisioning/agents?status=failed", status: http.StatusForbidden},
		{name: "filtered list", method: http.MethodGet, path: "/api/v1/agent-provisioning/agents?status=pending&q=secret", status: http.StatusForbidden},
		{name: "malformed list query", method: http.MethodGet, path: "/api/v1/agent-provisioning/agents?status=pending&%ZZ=secret", status: http.StatusForbidden},
		{name: "unrelated admin API", method: http.MethodGet, path: "/api/v1/admin/users", status: http.StatusForbidden},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, nil)
			request.Header.Set(agentProvisioningWorkerCredentialHeader, credential)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.status == http.StatusOK {
				require.Contains(t, response.Body.String(), `"auth_method":"agent_provisioning_worker"`)
				require.Contains(t, response.Body.String(), `"user_id":91`)
			}
		})
	}

	t.Run("invalid credential is rejected", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/agent-provisioning/agents/stream", nil)
		request.Header.Set(agentProvisioningWorkerCredentialHeader, "wrong")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("missing server credential fails closed", func(t *testing.T) {
		t.Setenv("AGENT_PROVISIONING_WORKER_CREDENTIAL", "")
		t.Setenv("AGENT_PROVISIONING_WORKER_CREDENTIAL_FILE", "")
		missingCredentialRouter := gin.New()
		missingCredentialRouter.Use(gin.HandlerFunc(NewAdminAuthMiddleware(nil, userService, nil, nil)))
		missingCredentialRouter.GET("/api/v1/agent-provisioning/agents/stream", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/agent-provisioning/agents/stream", nil)
		request.Header.Set(agentProvisioningWorkerCredentialHeader, credential)
		response := httptest.NewRecorder()
		missingCredentialRouter.ServeHTTP(response, request)
		require.Equal(t, http.StatusServiceUnavailable, response.Code)
	})
}

func TestLoadAgentProvisioningWorkerCredentialRequiresAbsoluteRegularFile(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "worker-credential")
	const fileCredential = "file-backed-worker-credential-with-minimum-length"
	if err := os.WriteFile(secretPath, []byte(fileCredential), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_PROVISIONING_WORKER_CREDENTIAL", "inline-fallback")
	t.Setenv("AGENT_PROVISIONING_WORKER_CREDENTIAL_FILE", secretPath)
	require.Equal(t, fileCredential, loadAgentProvisioningWorkerCredential())
	t.Setenv("AGENT_PROVISIONING_WORKER_CREDENTIAL_FILE", "relative-secret")
	require.Empty(t, loadAgentProvisioningWorkerCredential())
}

func TestProvisioningWorkerCredentialAuditMask(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/agent-provisioning/agents/stream", nil)
	ctx.Request.Header.Set(agentProvisioningWorkerCredentialHeader, "sensitive-worker-credential-value")
	masked := MaskedRequestCredential(ctx)
	require.NotContains(t, masked, "sensitive-worker-credential-value")
	require.Contains(t, masked, "agent_provisioning_worker")
}

type stubUserRepo struct {
	getByID       func(ctx context.Context, id int64) (*service.User, error)
	getFirstAdmin func(ctx context.Context) (*service.User, error)
}

func (s *stubUserRepo) Create(ctx context.Context, user *service.User) error {
	panic("unexpected Create call")
}

func (s *stubUserRepo) CreateWithEmailAliasGuard(ctx context.Context, user *service.User) error {
	panic("unexpected CreateWithEmailAliasGuard call")
}

func (s *stubUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	if s.getByID == nil {
		panic("GetByID not stubbed")
	}
	return s.getByID(ctx, id)
}

func (s *stubUserRepo) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	panic("unexpected GetByEmail call")
}

func (s *stubUserRepo) GetFirstAdmin(ctx context.Context) (*service.User, error) {
	if s.getFirstAdmin == nil {
		panic("unexpected GetFirstAdmin call")
	}
	return s.getFirstAdmin(ctx)
}

func (s *stubUserRepo) Update(ctx context.Context, user *service.User, fields service.UserUpdateFields) error {
	panic("unexpected Update call")
}

func (s *stubUserRepo) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (s *stubUserRepo) GetUserAvatar(ctx context.Context, userID int64) (*service.UserAvatar, error) {
	return nil, nil
}

func (s *stubUserRepo) UpsertUserAvatar(ctx context.Context, userID int64, input service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	panic("unexpected UpsertUserAvatar call")
}

func (s *stubUserRepo) DeleteUserAvatar(ctx context.Context, userID int64) error {
	panic("unexpected DeleteUserAvatar call")
}

func (s *stubUserRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *stubUserRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *stubUserRepo) GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs call")
}

func (s *stubUserRepo) GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID call")
}

func (s *stubUserRepo) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	panic("unexpected UpdateUserLastActiveAt call")
}

func (s *stubUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected UpdateBalance call")
}

func (s *stubUserRepo) DeductBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected DeductBalance call")
}

func (s *stubUserRepo) AdjustBalance(ctx context.Context, id int64, delta float64) (service.BalanceChange, error) {
	panic("unexpected AdjustBalance call")
}

func (s *stubUserRepo) SetBalance(ctx context.Context, id int64, value float64) (service.BalanceChange, error) {
	panic("unexpected SetBalance call")
}

func (s *stubUserRepo) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	panic("unexpected UpdateConcurrency call")
}

func (s *stubUserRepo) BatchSetConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (s *stubUserRepo) BatchAddConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (s *stubUserRepo) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	return 0, nil
}

func (s *stubUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	panic("unexpected ExistsByEmail call")
}

func (s *stubUserRepo) ExistsByEmailAlias(ctx context.Context, email string) (bool, error) {
	panic("unexpected ExistsByEmailAlias call")
}

func (s *stubUserRepo) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups call")
}

func (s *stubUserRepo) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups call")
}

func (s *stubUserRepo) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected AddGroupToAllowedGroups call")
}

func (s *stubUserRepo) ListUserAuthIdentities(ctx context.Context, userID int64) ([]service.UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities call")
}

func (s *stubUserRepo) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider call")
}

func (s *stubUserRepo) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	panic("unexpected UpdateTotpSecret call")
}

func (s *stubUserRepo) EnableTotp(ctx context.Context, userID int64) error {
	panic("unexpected EnableTotp call")
}

func (s *stubUserRepo) DisableTotp(ctx context.Context, userID int64) error {
	panic("unexpected DisableTotp call")
}

func (s *stubUserRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.User, error) {
	panic("unexpected GetByIDIncludeDeleted call")
}
