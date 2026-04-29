FROM node:24-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /web/build/ /src/internal/httpapi/webassets/

ARG VERSION=dev
ENV CGO_ENABLED=0
RUN go build \
        -trimpath \
        -ldflags "-s -w -X main.version=${VERSION}" \
        -o /out/bot \
        ./cmd/bot

FROM alpine:3.20
RUN addgroup -S -g 1000 bot && adduser -S -u 1000 -G bot bot \
 && mkdir -p /data && chown bot:bot /data
COPY --from=build /out/bot /bot
USER bot
ENTRYPOINT ["/bot"]
