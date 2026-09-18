FROM node:24-bookworm

WORKDIR /app

COPY package*.json ./
# The upstream lockfile currently omits a few Electron transitive packages;
# resolve from package.json so the deployment follows the checked-in manifest.
RUN npm install --ignore-scripts --no-audit --no-fund

COPY . .

ENV NODE_ENV=production
ENV HOME=/root
EXPOSE 5199

CMD ["./node_modules/.bin/vite", "--config", "config/vite.config.ts", "--host", "0.0.0.0"]
