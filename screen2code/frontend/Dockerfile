FROM node:22-bullseye-slim AS build

# Set the working directory in the container
WORKDIR /app

# Copy package.json and pnpm lockfile
COPY package.json pnpm-lock.yaml /app/

# Set the environment variable to skip Puppeteer download
ENV PUPPETEER_SKIP_DOWNLOAD=true

# Install dependencies
RUN corepack enable && pnpm install --frozen-lockfile

# Copy the current directory contents into the container at /app
COPY ./ /app/

ARG VITE_IS_DEPLOYED=true
ENV VITE_IS_DEPLOYED=${VITE_IS_DEPLOYED}
RUN pnpm build

FROM nginx:stable-alpine
ENV PROXY_CODEGEN_BACKEND=http://screen2code-backend:7001
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist/ /usr/share/nginx/html/
EXPOSE 5173
