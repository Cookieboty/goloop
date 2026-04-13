# syntax=docker/dockerfile:1

# Stage 1: Build Next.js frontend
FROM node:22-alpine AS frontend

WORKDIR /web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ .
RUN npm run build

# Verify static export produced the expected output
RUN test -f /web/out/index.html && test -d /web/out/_next

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Remove any stale embed directory, then copy fresh frontend build
RUN rm -rf ./internal/admin/out
COPY --from=frontend /web/out ./internal/admin/out

# Verify the _next directory is present before compiling
RUN ls ./internal/admin/out/_next/static/chunks/ | head -3

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /server ./cmd/server

# Stage 3: Minimal runtime image
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /server /app/server

RUN mkdir -p /tmp/images

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/server"]
