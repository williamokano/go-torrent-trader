# Stage 1: Build
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN [ -f go.sum ] && go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

# Stage 2: Run
FROM alpine:3.20
RUN apk --no-cache add ca-certificates && \
    adduser -D -H appuser
WORKDIR /app
COPY --from=builder /server /app/server
COPY --from=builder /app/migrations /app/migrations
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/server"]
