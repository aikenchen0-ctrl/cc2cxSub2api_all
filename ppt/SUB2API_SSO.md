# Sub2API sign-in and relay integration

The portal's Eternal PPT shortcut calls the authenticated
`GET /api/v1/auth/integrations/ppt/start` endpoint. PPT validates a signed,
two-minute ticket at `/api/v1/auth/sso/callback` and sets its existing HttpOnly
session cookie. Each portal subject maps to a separate ordinary PPT user;
existing local users, passwords, presentations, and administrator privileges
are not merged or replaced. Login by password remains available.

## Configuration

1. Complete PPT's initial administrator setup before enabling SSO.
2. Set the same random `SUB2API_SSO_SECRET` (at least 32 characters) in both
   services. Set the portal's `PPT_SSO_CALLBACK_URL` to the browser-accessible
   callback URL. Local deployment uses
   `http://localhost:8341/api/v1/auth/sso/callback`. Use HTTPS for public hosting.
3. In PPT, configure these server-only environment variables:

```dotenv
SUB2API_SSO_SECRET=<same-shared-secret>
SUB2API_BASE_URL=http://host.docker.internal:18080
SUB2API_API_KEY=<relay-service-api-key>
SUB2API_MODEL=gpt-5.4-mini
```

The relay key must have a funded, available route for the chosen model.
It is a shared instance provider: generation is billed to this service key,
not each user's portal key. A dedicated restricted key is recommended.
Never place API keys in the callback URL. The old query/header key override
has been removed because it changed settings for unrelated users.

Defaults are imported into the provider-settings database only when no LLM
provider has been selected. Existing administrator settings always win.
An existing installation can change the relay later in administrator settings;
changing environment defaults does not overwrite persisted configuration.
Image generation defaults to disabled unless already configured. Configure
an image provider separately when needed.

## Local Docker Deployment

Store the PPT variables above in the ignored `ppt/.env.sub2api` file, then run
from the repository root:

```powershell
docker build -t ppt:sub2api-integration -f ppt/Dockerfile ppt
docker compose -f ppt/docker-compose.yml -f ppt/docker-compose.sub2api.yml up -d --no-deps production
docker compose -f sub2api/deploy/docker-compose.yml -f sub2api/deploy/docker-compose.override.yml up -d --no-deps sub2api
```

Build the portal image `sub2api:ppt-integration` from `sub2api/Dockerfile` before
the last command. Back up the PPT data directory and portal database before
replacing services. Do not delete volumes during an upgrade.

## Verification

- Log into Sub2API and click Eternal PPT. A new tab must open at `/upload`
  without a second login or provider setup page.
- Reload the PPT page and confirm the ordinary session persists.
- Runtime config returns masked credentials; admin provider APIs return 403.
- Expired, altered, wrong-audience and replayed tickets must fail closed.
- Callback query strings must not appear in nginx or Uvicorn access logs.
- Run a small outline generation against the configured relay. Model listing
  alone does not prove an upstream can generate.

Unit/integration coverage lives in `tests/integration/test_sub2api_sso.py` and
`tests/unit/test_provider_settings.py` under `servers/fastapi`, and in the
portal's `backend/internal/handler/ppt_sso_test.go` (Go `unit` build tag).
