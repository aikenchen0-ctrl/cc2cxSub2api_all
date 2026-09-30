package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/satellite"
	"github.com/gin-gonic/gin"
)

const (
	satelliteOAuthBindingHandoffCookie = "satellite_oauth_binding_handoff"
	satelliteOAuthBindingCallbackPath  = "/auth/oauth/binding/callback"
)

type satelliteOAuthBindingStartRequest struct {
	Provider string `json:"provider"`
}

// SatelliteStartOAuthBinding creates a five-minute, provider-scoped browser
// handoff for the effective OBO user. The response contains an opaque signed
// token only; user JWTs and satellite credentials never enter the browser.
func (h *AuthHandler) SatelliteStartOAuthBinding(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.User == nil || apiKey.User.ID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	slug := strings.TrimSpace(c.GetHeader(middleware2.HeaderSatelliteApp))
	if _, exists := satellite.Lookup(slug); !exists {
		response.Unauthorized(c, "Invalid satellite identity")
		return
	}
	var req satelliteOAuthBindingStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	startPath, ok := satelliteOAuthBindingStartPath(provider)
	if !ok {
		response.BadRequest(c, "Unsupported identity provider")
		return
	}
	token, err := h.wechatPaymentResumeService().CreateOAuthBindingHandoffToken(service.OAuthBindingHandoffClaims{
		SatelliteSlug: slug,
		UserID:        apiKey.User.ID,
		Provider:      provider,
		RedirectTo:    "/profile",
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	query := url.Values{}
	query.Set("satellite_handoff", token)
	query.Set("redirect", "/profile")
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{
		"provider":      provider,
		"authorize_url": startPath + "?" + query.Encode(),
		"method":        http.MethodGet,
	})
}

func satelliteOAuthBindingStartPath(provider string) (string, bool) {
	switch provider {
	case "linuxdo", "oidc", "wechat", "dingtalk":
		return "/api/v1/auth/oauth/" + provider + "/bind/start", true
	default:
		return "", false
	}
}

func (h *AuthHandler) satelliteOAuthBindingClaims(c *gin.Context, provider string) (*service.OAuthBindingHandoffClaims, error) {
	raw := strings.TrimSpace(c.Query("satellite_handoff"))
	if raw == "" {
		return nil, nil
	}
	claims, err := h.wechatPaymentResumeService().ParseOAuthBindingHandoffToken(raw)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(provider), claims.Provider) {
		return nil, infraerrors.BadRequest("INVALID_OAUTH_BINDING_HANDOFF", "oauth binding provider does not match handoff")
	}
	if _, exists := satellite.Lookup(claims.SatelliteSlug); !exists {
		return nil, infraerrors.BadRequest("INVALID_OAUTH_BINDING_HANDOFF", "oauth binding satellite is not registered")
	}
	setCookie(c, satelliteOAuthBindingHandoffCookie, encodeCookieValue(raw), int(service.OAuthBindingHandoffTTL().Seconds()), isRequestHTTPS(c))
	return claims, nil
}

func (h *AuthHandler) satelliteOAuthBindingClaimsFromCookie(c *gin.Context, provider string) (*service.OAuthBindingHandoffClaims, error) {
	raw, err := readCookieDecoded(c, satelliteOAuthBindingHandoffCookie)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	claims, err := h.wechatPaymentResumeService().ParseOAuthBindingHandoffToken(raw)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(provider), claims.Provider) {
		return nil, infraerrors.BadRequest("INVALID_OAUTH_BINDING_HANDOFF", "oauth binding provider does not match handoff")
	}
	if _, exists := satellite.Lookup(claims.SatelliteSlug); !exists {
		return nil, infraerrors.BadRequest("INVALID_OAUTH_BINDING_HANDOFF", "oauth binding satellite is not registered")
	}
	return claims, nil
}

func clearSatelliteOAuthBindingHandoff(c *gin.Context) {
	clearCookie(c, satelliteOAuthBindingHandoffCookie, isRequestHTTPS(c))
}

func satelliteOAuthBindingCallbackURL(slug string) (string, error) {
	app, exists := satellite.Lookup(strings.TrimSpace(slug))
	if !exists {
		return "", errors.New("oauth binding satellite is not registered")
	}
	origin := satellite.ProjectOrigin(app.Slug, app.DefaultOrigin)
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("oauth binding satellite origin is invalid")
	}
	u.Path = satelliteOAuthBindingCallbackPath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func (h *AuthHandler) satelliteOAuthBindingFrontendCallback(c *gin.Context, provider, fallback string) (*service.OAuthBindingHandoffClaims, string) {
	claims, err := h.satelliteOAuthBindingClaimsFromCookie(c, provider)
	if err != nil || claims == nil {
		return nil, fallback
	}
	callbackURL, err := satelliteOAuthBindingCallbackURL(claims.SatelliteSlug)
	if err != nil {
		return nil, fallback
	}
	return claims, callbackURL
}

func (h *AuthHandler) completeSatelliteOAuthBinding(
	c *gin.Context,
	claims *service.OAuthBindingHandoffClaims,
	identity service.PendingAuthIdentityKey,
	resolvedEmail string,
	upstreamClaims map[string]any,
) error {
	if claims == nil || claims.UserID <= 0 || !strings.EqualFold(claims.Provider, identity.ProviderType) {
		return infraerrors.BadRequest("INVALID_OAUTH_BINDING_HANDOFF", "oauth binding handoff is invalid")
	}
	user, err := h.userService.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		return err
	}
	if err := ensureLoginUserActive(user); err != nil {
		return err
	}
	targetUserID := claims.UserID
	pending := &dbent.PendingAuthSession{
		Intent:                 oauthIntentBindCurrentUser,
		ProviderType:           identity.ProviderType,
		ProviderKey:            identity.ProviderKey,
		ProviderSubject:        identity.ProviderSubject,
		TargetUserID:           &targetUserID,
		ResolvedEmail:          strings.TrimSpace(resolvedEmail),
		RedirectTo:             strings.TrimSpace(claims.RedirectTo),
		UpstreamIdentityClaims: upstreamClaims,
	}
	return applyPendingOAuthBinding(c.Request.Context(), h.entClient(), h.authService, h.userService, pending, nil, &targetUserID, true, true)
}

func redirectSatelliteOAuthBindingSuccess(c *gin.Context, callbackURL, provider, redirectTo string) {
	fragment := url.Values{}
	fragment.Set("status", "success")
	fragment.Set("provider", strings.ToLower(strings.TrimSpace(provider)))
	if redirect := sanitizeFrontendRedirectPath(redirectTo); redirect != "" {
		fragment.Set("redirect", redirect)
	}
	redirectWithFragment(c, callbackURL, fragment)
}
