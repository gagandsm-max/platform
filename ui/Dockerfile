# ==============================================================================
# Platform UI Dockerfile (Clean-room Multi-stage Build: React + Nginx HTTPS-only)
# ==============================================================================

FROM node:20-alpine AS builder
WORKDIR /app

# Copy dependency manifests
COPY package.json ./
RUN npm install

# Copy source code and build production bundle
COPY tsconfig.json vite.config.ts index.html ./
COPY src ./src
RUN npm run build

# ==============================================================================
# Runtime Stage: Nginx HTTPS-only Reverse Proxy
# ==============================================================================
FROM nginx:1.27-alpine
WORKDIR /usr/share/nginx/html

RUN apk add --no-cache curl \
    && mkdir -p /etc/nginx/certs \
    && rm -rf /etc/nginx/conf.d

# Copy custom HTTPS-only Nginx configuration (No Port 80 Listener)
COPY nginx.conf /etc/nginx/nginx.conf

# Copy production frontend build
COPY --from=builder /app/dist /usr/share/nginx/html

# Expose HTTPS port only (Port 80 is strictly disallowed)
EXPOSE 443

HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -k -f https://127.0.0.1:443/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
