FROM node:22-bookworm-slim

WORKDIR /app
COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

ENV API_HOST=0.0.0.0 \
    API_PORT=5173 \
    NODE_ENV=production
EXPOSE 5173

CMD ["node", "server.mjs"]
