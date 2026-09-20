package satellite

import "net/http"

// Common headers for satellite projects opened from the Sub2API sidebar.
// The browser never sees a Super Key. The project backend sends the shared
// app credential and the logged-in Sub2API user id; Sub2API then uses that
// user's Super Key internally to reach every active upstream group.
const (
	HeaderOnBehalfOf = "X-Sub2API-On-Behalf-Of"
	HeaderApp        = "X-Sub2API-Satellite"
)

// SetRequestHeaders marks a /v1 call as a satellite on-behalf-of request.
func SetRequestHeaders(header http.Header, appCredential, slug, userID string) {
	if header == nil {
		return
	}
	header.Set("Authorization", "Bearer "+appCredential)
	header.Set(HeaderOnBehalfOf, userID)
	header.Set(HeaderApp, slug)
}
