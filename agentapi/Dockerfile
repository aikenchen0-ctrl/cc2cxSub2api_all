# AgentAPI is built as one self-contained image. The frontend is compiled into
# backend/web and served by the Go process, so deployment only needs one port.
FROM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
RUN corepack enable
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm run build

FROM golang:1.27-bookworm AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agentapi . \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agentapi-provision ./cmd/agentapi-provision

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && addgroup -S agentapi && adduser -S -G agentapi agentapi
WORKDIR /app
COPY --from=backend /out/agentapi /app/agentapi
COPY --from=backend /out/agentapi-provision /app/agentapi-provision
COPY --from=frontend /src/backend/web /app/web
RUN mkdir -p /app/data && chown -R agentapi:agentapi /app
USER agentapi
STOPSIGNAL SIGTERM
ENV AGENTAPI_ADDR=:8080 \
    AGENTAPI_WEB_DIR=/app/web \
    AGENTAPI_DATABASE_PATH=/app/data/agentapi.db
EXPOSE 8080
VOLUME ["/app/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/agentapi"]
