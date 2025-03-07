FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o bot .

# Create a minimal production image
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/bot .
# Copy any necessary data files
COPY --from=builder /app/tutorial_data.json .

# Create directory for user logs
RUN mkdir -p user_logs

# Command to run the executable
CMD ["./bot"]
