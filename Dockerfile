FROM golang:1.26-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /usr/local/bin/driftr ./cmd/driftr/

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/driftr /usr/local/bin/driftr

# Create a non-root user for testing
RUN useradd -m -s /bin/bash driftr
USER driftr
WORKDIR /home/driftr

ENTRYPOINT ["driftr"]
