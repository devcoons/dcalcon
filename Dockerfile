# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26
ARG NODE_VERSION=22

FROM golang:${GO_VERSION}-bookworm AS go-build
WORKDIR /src
ENV CGO_ENABLED=0
COPY src/svc/go.mod src/svc/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY src/svc/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/dcalcon ./cmd/dcalcon

FROM node:${NODE_VERSION}-bookworm-slim AS web-build
WORKDIR /web
COPY src/web/package.json src/web/package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm npm ci || npm install
COPY src/web/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM debian:bookworm-slim AS core
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata wget util-linux \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 1000 --home-dir /data --shell /usr/sbin/nologin dcalcon \
    && mkdir -p /data \
    && chown dcalcon:dcalcon /data
COPY --from=go-build /out/dcalcon /dcalcon
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod 755 /docker-entrypoint.sh
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["serve"]

FROM core AS caldav
CMD ["caldav"]

FROM core AS carddav
CMD ["carddav"]

FROM core AS api
CMD ["api"]

FROM core AS worker
CMD ["worker"]

FROM node:${NODE_VERSION}-bookworm-slim AS web
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
RUN useradd --system --uid 1001 nextjs
COPY --from=web-build /web/public ./public
COPY --from=web-build --chown=nextjs:nextjs /web/.next/standalone ./
COPY --from=web-build --chown=nextjs:nextjs /web/.next/static ./.next/static
COPY --from=web-build --chown=nextjs:nextjs /web/runtime-proxy.mjs ./runtime-proxy.mjs
USER nextjs
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=25s --retries=3 \
  CMD node -e "require('http').get('http://127.0.0.1:3000/login',r=>{r.resume();process.exit(r.statusCode<500?0:1)}).on('error',()=>process.exit(1))"
CMD ["node", "runtime-proxy.mjs"]
