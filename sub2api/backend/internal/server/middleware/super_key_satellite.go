package middleware

import (
	"context"
	"crypto/hmac"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	// HeaderOnBehalfOf is the Sub2API user id the satellite is calling for.
	HeaderOnBehalfOf = "X-Sub2API-On-Behalf-Of"
	// HeaderSatelliteApp identifies the calling project (canvas, aicut, ...).
	HeaderSatelliteApp = "X-Sub2API-Satellite"
	// HeaderAppCredential is accepted only as a legacy extra header.
	// New satellite calls put the app credential in Authorization.
	HeaderAppCredential = "X-Sub2API-App-Credential"
	envAppCredential    = "SUB2API_APP_CREDENTIAL"
)

var errSatelliteUser = errors.New("invalid satellite user")

func satelliteAppCredential() string {
	return strings.TrimSpace(os.Getenv(envAppCredential))
}

// SatelliteBearerAccepted reports whether Authorization is the project
// credential, not a user API key and not a Super Key.
func SatelliteBearerAccepted(presented string) bool {
	expected := satelliteAppCredential()
	if expected == "" {
		return false
	}
	presented = strings.TrimSpace(presented)
	if presented == "" {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(presented))
}

func parseOnBehalfOfUserID(c *gin.Context) (int64, error) {
	if c == nil {
		return 0, errSatelliteUser
	}
	raw := strings.TrimSpace(c.GetHeader(HeaderOnBehalfOf))
	if raw == "" || len(raw) > 20 {
		return 0, errSatelliteUser
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errSatelliteUser
	}
	return id, nil
}

func satelliteSlugOK(c *gin.Context) bool {
	if c == nil {
		return false
	}
	slug := strings.TrimSpace(c.GetHeader(HeaderSatelliteApp))
	if slug == "" || len(slug) > 32 {
		return false
	}
	for _, r := range slug {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func rejectSuperKeyAsAPIKey(c *gin.Context, presented string) bool {
	if !service.IsSuperAPIKeyCredential(presented) {
		return false
	}
	AbortWithError(c, http.StatusForbidden, "SUPER_KEY_NOT_AN_API_KEY", "Super Key cannot be used as an API key")
	return true
}

// bindSatelliteSuperKeyContext marks satellite on-behalf-of traffic as a
// cross-group Super Key request. SatelliteUserKey is a Super Key even when
// IsSuper() cannot see the sk-super- prefix; without this flag the scheduler
// treats group_id=null as "ungrouped accounts only" and chat/images return 503.
func bindSatelliteSuperKeyContext(c *gin.Context, satelliteHandled bool, apiKey *service.APIKey) {
	if c == nil || c.Request == nil || apiKey == nil {
		return
	}
	if !satelliteHandled && !apiKey.IsSuper() {
		return
	}
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.SuperAPIKey, true))
}

func loadSatelliteUserKey(c *gin.Context, apiKeyService *service.APIKeyService, presented string) (*service.APIKey, bool) {
	if !SatelliteBearerAccepted(presented) {
		return nil, false
	}
	if !satelliteSlugOK(c) {
		AbortWithError(c, http.StatusUnauthorized, "SATELLITE_APP_REQUIRED", "Satellite app header is required")
		return nil, true
	}
	userID, err := parseOnBehalfOfUserID(c)
	if err != nil {
		AbortWithError(c, http.StatusUnauthorized, "SATELLITE_USER_REQUIRED", "X-Sub2API-On-Behalf-Of must be the Sub2API user id")
		return nil, true
	}
	if apiKeyService == nil {
		AbortWithError(c, http.StatusServiceUnavailable, "SATELLITE_UNAVAILABLE", "Satellite Super Key service is unavailable")
		return nil, true
	}
	key, err := apiKeyService.SatelliteUserKey(c.Request.Context(), userID)
	if err != nil || key == nil {
		AbortWithError(c, http.StatusUnauthorized, "SATELLITE_USER_UNAVAILABLE", "Unable to resolve satellite user")
		return nil, true
	}
	return key, true
}
