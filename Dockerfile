# Dockerfile for Stacker - Service Supervisor
# Supports both Debian (glibc) and Alpine (musl) based images
#
# Build for Debian (default):
#   docker build -t stacker:debian .
#
# Build for Alpine:
#   docker build --build-arg DISTRO=alpine -t stacker:alpine .

# Define the distro argument with debian as default
ARG DISTRO=debian

# Define the base images for building the Go application
# debian-builder: Uses Debian which is based on glibc
# alpine-builder: Uses Alpine which is based on musl libc
FROM golang:1.20 AS builder-debian
FROM golang:1.20-alpine AS builder-alpine

# Build stage - uses the specified distro for the build environment
# This ensures that the Go application is built with the appropriate libc
FROM builder-${DISTRO} AS builder

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
# Set CGO_ENABLED=0 for a static binary that works in minimal images
RUN CGO_ENABLED=0 go build -o stacker -ldflags="-s -w" .

# Runtime stage
# The DISTRO ARG needs to be redeclared in this stage because ARGs don't persist across FROM instructions
ARG DISTRO=debian

# Define the runtime images for each supported distribution
# debian-runtime: Uses Debian slim which is based on glibc
# alpine-runtime: Uses Alpine which is based on musl libc
FROM debian:stable-slim AS debian-runtime
FROM alpine:latest AS alpine-runtime

# Select the appropriate runtime image based on the DISTRO argument
# This allows us to create a minimal image with just the binary and its dependencies
FROM ${DISTRO}-runtime

# Create directory for configuration
RUN mkdir -p /usr/local/etc/stacker

# Copy the binary from the builder stage
COPY --from=builder /app/stacker /usr/local/bin/stacker

# Default command (can be overridden completely)
CMD ["/usr/local/bin/stacker", "--help"]

# Document that the service listens on port 8080 by default
EXPOSE 8080

# Document volumes
VOLUME ["/usr/local/etc/stacker"]
