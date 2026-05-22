# syntax=docker/dockerfile:1.7

# --- stage 1: build ---
FROM node:20-bookworm-slim AS deps
WORKDIR /app
COPY services/console/package.json services/console/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci --omit=dev=false; else npm install; fi

FROM node:20-bookworm-slim AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY services/console/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# --- stage 2: runtime (standalone) ---
FROM node:20-bookworm-slim AS runtime
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=8090 HOSTNAME=0.0.0.0

# Non-root user (Next.js standalone runs fine as nonroot).
RUN addgroup --system --gid 1001 aegis && \
    adduser --system --uid 1001 --ingroup aegis aegis

COPY --from=build --chown=aegis:aegis /app/public ./public
COPY --from=build --chown=aegis:aegis /app/.next/standalone ./
COPY --from=build --chown=aegis:aegis /app/.next/static ./.next/static

USER 1001:1001
EXPOSE 8090
ENTRYPOINT ["node", "server.js"]
