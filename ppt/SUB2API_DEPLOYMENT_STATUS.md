# Local Deployment Verification

Verified on September 18, 2026.

## Deployed

- Portal: http://localhost:18080, image sub2api:ppt-integration.
- PPT: http://localhost:8341, image ppt:sub2api-integration.
- Portal shortcut uses authenticated /api/v1/auth/integrations/ppt/start.
- Callback: http://localhost:8341/api/v1/auth/sso/callback.
- Server-side default relay: http://host.docker.internal:18080/v1.
- Default text model: gpt-5.4-mini. Existing image generation remains disabled.
- Portal and PPT share the signing secret. No permanent key is in the URL.
- The relay currently uses the existing administrator-owned programming key;
  usage is charged to that shared key, not individual portal users.

## Verified

- 40 Python authentication, SSO, and provider-settings tests passed.
- Portal PPT/Canvas/Ju/Livart SSO handler tests passed with the unit tag.
- Both production Docker images built successfully.
- Isolated real portal browser login and Eternal PPT click reach /upload
  without another login or model setup; reload preserves the ordinary session.
- Runtime config masks credentials; ordinary users cannot read admin settings.
- Separate users cannot read each other's test presentation; replay fails.
- Callback tickets do not appear in tested container logs.
- The deployed callback, shared-secret configuration, runtime config, and page
  reload passed in an actual Chromium browser. Desktop/mobile screenshots saved.
- Existing PPT data preserved: one administrator and zero presentations before
  and after the smoke test. Synthetic test accounts and artifacts removed.

## Remaining External Failure

Real PPT outline generation reached this Sub2API instance's chat-completions
endpoint but returned an SSE error: all available accounts are currently
rate-limited. Earlier small relay probes found upstream insufficient balance,
model-not-found, and service-unavailable responses. Model listing works but is
not proof that generation works. No upstream balances or disabled channels
were changed. Successful real outline generation remains unverified until a
usable funded upstream route is available for the configured model.

Follow-up checks on September 18 confirmed the upstream model catalog differs
from the gateway's saved catalog: the currently enabled upstream lists only
codex-auto-review, gpt-5.5, gpt-5.6-sol, and gpt-6-astra. In particular, the
configured gpt-5.4-mini default is not currently served there. A small gpt-5.5
request failed through Sub2API (502) and directly against the same configured
upstream (503, Service temporarily unavailable), ruling out PPT request
formatting as the cause of that failure. No additional model probes are
needed until upstream availability changes. Before re-running the real outline
test, select an actually supported, working model in PPT administrator
settings; startup environment defaults do not overwrite persisted settings.

## Backup and Evidence

Backup directory (relative to the workspace root):
sub2api/deploy/migration-backups/ppt-sso-20260918-110627/.

It contains a PostgreSQL custom-format dump, the portal environment backup,
a stopped-service copy of ppt-app-data, and browser screenshots under
browser-isolated/ and browser-deployed/. Backups include credentials and
must remain private. Screenshots were saved; this environment did not support
visual image inspection, so UI evidence is based on browser DOM assertions.

Previous images remain available: sub2api:canvas-integration and
ppt-production:latest. Rollback requires selecting the old images and, if
rolling back provider/data state, restoring the matching backup while PPT is
stopped. Do not delete volumes or overwrite live databases during rollback.

## Repeatable Browser Checks

scripts/sub2api-sso-e2e.cjs boots isolated real portal/PPT containers and tests
browser login, the shortcut, masked defaults, session persistence, ticket
replay, and cross-user presentation access. It does not test real generation.
Set PLAYWRIGHT_MODULE to an installed Playwright module path when needed.

scripts/sub2api-deployed-smoke.cjs checks the deployed local services using a
temporary synthetic ordinary identity, then makes a real outline request. It
removes only its own test identity and presentation. Exit code 2 means the UI
and SSO worked but the relay/upstream could not generate; HTTP 200 on the SSE
endpoint by itself is not a generation success.
