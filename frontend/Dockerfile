# syntax=docker/dockerfile:1

# --- build ---------------------------------------------------------------
FROM node:24-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# Relative by default: nginx proxies /api to the backend, so the browser only
# ever talks to one origin.
ARG VITE_API_BASE=/api/v1
ENV VITE_API_BASE=${VITE_API_BASE}

RUN npm run build

# --- runtime -------------------------------------------------------------
FROM nginx:1.31-alpine

# The upstream is templated so the same image works under compose (service
# name "backend") and Kubernetes (a Service DNS name).
ENV BACKEND_HOST=backend
ENV BACKEND_PORT=8080

# The Authorization header nginx presents when proxying /api, whole: "Bearer
# <token>". Empty means the browser supplies its own, which is what the token
# gate is for. It has to be declared even when empty -- the entrypoint only
# substitutes variables that already exist, and an undeclared one would
# survive into the rendered config as literal text.
ENV API_AUTHORIZATION=""

COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/templates/default.conf.template
COPY nginx-security-headers.conf /etc/nginx/snippets/security-headers.conf

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost/healthz || exit 1
