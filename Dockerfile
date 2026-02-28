# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /watch-dog ./cmd/watch-dog

# Runtime stage (minimal)
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /watch-dog /watch-dog
CMD ["/watch-dog"]
