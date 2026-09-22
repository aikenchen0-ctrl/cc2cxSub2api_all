FROM node:22-bookworm-slim

WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .

ENV VITE_PORT=3000
EXPOSE 3000
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
