# The build output is a static site, so build it once on the runner's native
# platform instead of under QEMU emulation; only the nginx stage below is
# per-architecture.
FROM --platform=$BUILDPLATFORM docker.io/library/node:24.16.0-alpine3.22 AS mermaid-live-editor-dependencies

RUN apk --no-cache add build-base git python3 && \
    rm -rf /var/cache/apk/*

RUN corepack enable pnpm

WORKDIR /app

COPY ./package.json .
COPY ./pnpm-lock.yaml .

RUN pnpm install

FROM mermaid-live-editor-dependencies AS mermaid-live-editor-builder

ARG MERMAID_RENDERER_URL
ARG MERMAID_KROKI_RENDERER_URL
ARG MERMAID_ANALYTICS_URL
ARG MERMAID_DOMAIN
ARG MERMAID_IS_ENABLED_MERMAID_CHART_LINKS
ARG MERMAID_PRIVACY_POLICY_URL
ARG MERMAID_HIDE_PRIVACY_POLICY
ARG MERMAID_BASE_PATH

COPY . ./

RUN pnpm build

FROM mermaid-live-editor-builder AS mermaid-dev

ENTRYPOINT ["pnpm", "dev"]

FROM nginx:1.28-alpine3.21 AS mermaid

EXPOSE 8080

COPY ./nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=mermaid-live-editor-builder /app/docs /usr/share/nginx/html

FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.26-alpine AS storage-api-builder

WORKDIR /src

COPY server-go/go.mod server-go/go.sum ./
RUN go mod download

COPY server-go/ ./
RUN CGO_ENABLED=0 go build -o /out/mermaid-storage .

FROM docker.io/library/alpine:3.22 AS storage-api

ENV HOST=0.0.0.0
WORKDIR /app
COPY --from=storage-api-builder /out/mermaid-storage /usr/local/bin/mermaid-storage
VOLUME /app/server-data
EXPOSE 18790
CMD ["mermaid-storage"]
