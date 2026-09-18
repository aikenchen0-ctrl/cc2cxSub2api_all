# Docker deployment

Run `docker compose up -d --build` from this directory, then open
http://localhost:4173. The container restarts automatically with Docker.

This deploys the project's static edition. Workbooks are processed in the
browser. Configure a DeepSeek API key in the application's settings to use AI.
The key is stored in the browser, not in the Docker image.

`AIEXCEL_PORT` (default `4173`) and `AIEXCEL_BIND_HOST` (default `0.0.0.0`)
configure the published endpoint.

The Sub2API AI Spreadsheet menu opens port 4173 on the current browser host.
For a custom domain or HTTPS reverse proxy, set `VITE_AIEXCEL_URL` in the
Sub2API frontend build environment and rebuild its image.
