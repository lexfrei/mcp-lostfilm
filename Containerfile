FROM docker.io/library/golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder

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
