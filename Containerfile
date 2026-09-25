FROM docker.io/library/golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

ARG VERSION=development
ARG REVISION=unknown

# hadolint ignore=DL3018
RUN echo 'nobody:x:65534:65534:Nobody:/home/nobody:' > /tmp/passwd && \
    apk add --no-cache ca-certificates && \
    mkdir -p /home/nobody/.mcp-lostfilm && chown 65534:65534 /home/nobody/.mcp-lostfilm

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.revision=${REVISION}" -trimpath ./cmd/mcp-lostfilm

FROM scratch

COPY --from=builder /tmp/passwd /etc/passwd
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder --chmod=555 /build/mcp-lostfilm /mcp-lostfilm
COPY --from=builder --chown=65534:65534 /home/nobody/.mcp-lostfilm /home/nobody/.mcp-lostfilm

ENV LOSTFILM_COOKIE_FILE=/home/nobody/.mcp-lostfilm/cookies.json

USER 65534
ENTRYPOINT ["/mcp-lostfilm"]
