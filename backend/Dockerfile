# Stage 1: Build
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN [ -f go.sum ] && go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

# Stage 2: Run
# postgresql16-client provides pg_dump for the admin database backup feature
# (BE-8.7). Keep the major version in step with the postgres server image.
FROM alpine:3.20
RUN apk --no-cache add ca-certificates postgresql16-client && \
    adduser -D -H appuser
WORKDIR /app
COPY --from=builder /server /app/server
COPY --from=builder /app/migrations /app/migrations
# Backup destination (BACKUP_DIR). Mount a volume here to keep backups across
# container restarts.
RUN mkdir -p /app/backups && chown appuser /app/backups
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/server"]
