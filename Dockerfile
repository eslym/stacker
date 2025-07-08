FROM alpine:latest

ARG GOARCH=amd64

# Create directory for configuration
RUN mkdir -p /usr/local/etc/stacker

# Copy the binary from the builder stage
COPY ./build/stacker-${GOARCH} /usr/local/bin/stacker

# Default command (can be overridden completely)
CMD ["/usr/local/bin/stacker"]

# Document that the service listens on port 8080 by default
EXPOSE 8080

# Document volumes
VOLUME ["/usr/local/etc/stacker"]
