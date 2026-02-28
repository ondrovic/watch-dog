# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /watch-dog ./cmd/watch-dog

# Runtime stage (minimal)
FROM alpine:3.19
RUN apk --no-cache add ca-certificates docker-cli
COPY --from=builder /watch-dog /watch-dog
HEALTHCHECK --interval=15s --start-period=20s --timeout=10s --retries=2 CMD docker info
CMD ["/watch-dog"]
