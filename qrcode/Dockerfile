FROM node:24-alpine

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci --omit=dev

COPY src ./src
COPY public ./public

ENV NODE_ENV=production \
    PORT=5221 \
    QR_CORS_ORIGIN=http://localhost:5221 \
    QR_API_BASE_URL=http://localhost:18080

EXPOSE 5221

CMD ["node", "src/server.js"]
