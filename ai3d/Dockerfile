FROM node:22-bookworm-slim

WORKDIR /app
COPY package*.json ./
RUN npm ci

COPY . .

ENV API_HOST=0.0.0.0 \
    API_PORT=8787 \
    VITE_PORT=5173 \
    NODE_ENV=production
EXPOSE 5173 5174 8787

# The Vite UI proxies /api to the colocated Node API. Keeping both processes
# in one small satellite container also keeps the browser on one origin.
CMD ["sh", "-c", "node server.mjs & exec npm run dev -- --host 0.0.0.0"]
