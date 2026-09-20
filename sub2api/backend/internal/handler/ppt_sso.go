package handler

import "net/url"

func validPPTCallback(u *url.URL) bool {
	if u == nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.Fragment != "" || u.ForceQuery ||
		u.Path != "/api/v1/auth/sso/callback" || u.RawPath != "" {
		return false
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	return u.Scheme == "https" || (u.Scheme == "http" && local)
}
