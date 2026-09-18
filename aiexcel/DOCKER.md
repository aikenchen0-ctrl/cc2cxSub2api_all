# Docker deployment

Run `docker compose up -d --build` from this directory, then open
http://localhost:4173. The container restarts automatically with Docker.

This deploys the server edition. Workbooks are still processed in the browser,
but SSO callbacks and AI requests are handled by the server. Configure
`SUB2API_SSO_SECRET`, `SUB2API_RELAY_BASE_URL`, and
`SUB2API_RELAY_API_KEY` in the container environment. The browser never stores
or receives the relay API key.

`AIEXCEL_PORT` (default `4173`) and `AIEXCEL_BIND_HOST` (default `0.0.0.0`)
configure the published endpoint.

The Sub2API AI Spreadsheet menu opens port 4173 on the current browser host.
For a custom domain or HTTPS reverse proxy, set `VITE_AIEXCEL_URL` in the
Sub2API frontend build environment and rebuild its image.
