# LibriNode — multi-stage build: web UI, then the Go binary that embeds it.
#
#   docker build -t librinode .
#   docker run -d -p 7845:7845 \
#     -v /path/to/config:/config -v /path/to/media:/media librinode
#
# The container runs as UID/GID 1000 by default; override with PUID/PGID.

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS build
ARG VERSION=docker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /librinode ./cmd/librinode

FROM alpine:3.20
RUN apk add --no-cache ca-certificates su-exec tzdata \
    && addgroup -g 1000 librinode \
    && adduser -u 1000 -G librinode -D -H librinode
COPY --from=build /librinode /usr/local/bin/librinode
COPY packaging/docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV PUID=1000 PGID=1000
VOLUME /config
EXPOSE 7845
ENTRYPOINT ["/entrypoint.sh"]
CMD ["--data", "/config"]
