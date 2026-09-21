package satellite

import "time"

// App is one sidebar satellite. New projects add a row here; do not copy SSO handlers.
type App struct {
	Slug          string
	Audience      string
	DefaultOrigin string
	CallbackPath  string
	CallbackEnv   string
	DefaultNext   string
	TicketTTL     time.Duration
}

func defaultTTL() time.Duration { return 2 * time.Minute }

// Apps is the catalog used by SSO start and {slug}_link.
func Apps() []App {
	ttl := defaultTTL()
	return []App{
		{Slug: "canvas", Audience: "canvas", DefaultOrigin: "http://localhost:3522", CallbackPath: "/api/auth/sso/callback", CallbackEnv: "CANVAS_SSO_CALLBACK_URL", DefaultNext: "/", TicketTTL: ttl},
		{Slug: "ju", Audience: "ju", DefaultOrigin: "http://localhost:3000", CallbackPath: "/api/auth/sso/callback", CallbackEnv: "JU_SSO_CALLBACK_URL", DefaultNext: "/projects", TicketTTL: 3 * 24 * time.Hour},
		{Slug: "livart", Audience: "livart", DefaultOrigin: "http://localhost:8080", CallbackPath: "/api/auth/sso/callback", CallbackEnv: "LIVART_SSO_CALLBACK_URL", DefaultNext: "/", TicketTTL: ttl},
		{Slug: "ppt", Audience: "presenton", DefaultOrigin: "http://localhost:8341", CallbackPath: "/api/v1/auth/sso/callback", CallbackEnv: "PPT_SSO_CALLBACK_URL", DefaultNext: "/upload", TicketTTL: ttl},
		{Slug: "aicut", Audience: "openchatcut", DefaultOrigin: "http://localhost:5199", CallbackPath: "/api/auth/sso/callback", CallbackEnv: "OPENCHATCUT_SSO_CALLBACK_URL", DefaultNext: "/", TicketTTL: ttl},
		{Slug: "screen2code", Audience: "screen2code", DefaultOrigin: "http://localhost:5173", CallbackPath: "/api/auth/sso/callback", CallbackEnv: "SCREEN2CODE_SSO_CALLBACK_URL", DefaultNext: "/", TicketTTL: ttl},
		{Slug: "aiexcel", Audience: "aiexcel", DefaultOrigin: "http://localhost:4173", CallbackPath: "/api/auth/sso/callback", CallbackEnv: "AIEXCEL_SSO_CALLBACK_URL", DefaultNext: "/", TicketTTL: ttl},
		{Slug: "qrcode", Audience: "qrcode", DefaultOrigin: "http://localhost:5221", CallbackPath: "/api/auth/sso/callback", CallbackEnv: "QRCODE_SSO_CALLBACK_URL", DefaultNext: "/", TicketTTL: ttl},
		{Slug: "yibiao", Audience: "yibiao", DefaultOrigin: "http://localhost:8080", CallbackPath: "/api/auth/sso/callback", CallbackEnv: "YIBIAO_SSO_CALLBACK_URL", DefaultNext: "/", TicketTTL: ttl},
	}
}

func Lookup(slug string) (App, bool) {
	for _, app := range Apps() {
		if app.Slug == slug {
			return app, true
		}
	}
	return App{}, false
}
