package satellite

import (
	"net/url"
	"os"
	"strings"
)

// Env names from 本地链接.txt:
//
//	LINK              model gateway (localhost:18080 or https://api.cc2.cx)
//	{project}_link    sidebar / SSO origin (http://localhost:PORT or https://{project}.cc2.cx)

func envValue(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func isLocalHost(host string) bool {
	h := strings.ToLower(host)
	return h == "localhost" || h == "127.0.0.1" || h == "0.0.0.0" || h == "::1"
}

// NormalizeOrigin turns LINK / *_link values into an origin.
// Bare localhost values get http://; other bare hosts get https://.
func NormalizeOrigin(raw, fallback string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		host := value
		if i := strings.IndexByte(value, '/'); i >= 0 {
			host = value[:i]
		}
		if colon := strings.IndexByte(host, ':'); colon >= 0 {
			host = host[:colon]
		}
		if isLocalHost(host) {
			value = "http://" + value
		} else {
			value = "https://" + value
		}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	return strings.TrimRight(value, "/")
}

// ProjectOrigin is the public origin of a sibling app from {PROJECT}_LINK or {project}_link.
func ProjectOrigin(project, fallback string) string {
	project = strings.TrimSpace(project)
	return NormalizeOrigin(envValue(strings.ToUpper(project)+"_LINK", strings.ToLower(project)+"_link"), fallback)
}

// ModelOrigin is the Sub2API gateway origin from LINK.
func ModelOrigin(fallback string) string {
	return NormalizeOrigin(envValue("LINK", "SUB2API_RELAY_BASE_URL", "SUB2API_BASE"), fallback)
}

// CallbackURL is the SSO callback. Explicit *_SSO_CALLBACK_URL wins;
// otherwise it is {project}_link + path.
func CallbackURL(envName, project, fallbackOrigin, path string) string {
	if explicit := strings.TrimSpace(os.Getenv(envName)); explicit != "" {
		return strings.TrimRight(explicit, "/")
	}
	origin := ProjectOrigin(project, fallbackOrigin)
	if origin == "" {
		return ""
	}
	if path == "" {
		return origin
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return origin + path
}
