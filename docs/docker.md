# Stacker Docker Usage Guide

Stacker provides official Docker images for easy deployment in containerized environments. This guide covers image tags, usage patterns, and best practices.

## Available Images
- `eslym/stacker:latest` (Alpine-based, minimal, recommended)
- Versioned tags: `eslym/stacker:x.y.z`
- Multi-arch: Supports amd64 and arm64

> **Note:** The image is built with a fully static Go binary and runs on any Linux distribution. There is no longer a separate Debian or Alpine variant.

## Basic Usage
```bash
docker run -v /path/to/config:/usr/local/etc/stacker eslym/stacker
```

## Exposing the Admin Interface
```bash
docker run -v /path/to/config:/usr/local/etc/stacker -p 8080:8080 eslym/stacker
```

## Using a Custom Configuration File
```bash
docker run -v /path/to/stacker.yaml:/stacker.yaml eslym/stacker --config /stacker.yaml
```

## Running Specific Services
```bash
docker run eslym/stacker nginx php-fpm
docker run eslym/stacker --with=horizon
docker run eslym/stacker --except scheduler
```

## Docker Compose Example
```yaml
version: "3"
services:
  stacker:
    image: eslym/stacker:latest
    volumes:
      - ./config:/usr/local/etc/stacker
    ports:
      - "8080:8080"
    restart: unless-stopped
    command: --verbose
```

## Building Custom Images
```Dockerfile
FROM eslym/stacker:latest
COPY stacker.yaml /usr/local/etc/stacker/stacker.yaml
CMD ["/usr/local/bin/stacker", "--verbose"]
```

## Using the Stacker Binary in Your Own Image

You can use the official Stacker image as a build source to copy only the binary into your own custom image. This is useful for minimal images or multi-stage builds:

```Dockerfile
FROM eslym/stacker:latest as stacker-binary

FROM alpine:latest
COPY --from=stacker-binary /usr/local/bin/stacker /usr/local/bin/stacker
COPY stacker.yaml /usr/local/etc/stacker/stacker.yaml
CMD ["/usr/local/bin/stacker", "--config", "/usr/local/etc/stacker/stacker.yaml"]
```

This approach ensures you always get the latest, tested Stacker binary and can build a minimal runtime image with your own configuration and dependencies.

## Best Practices
- Use volumes for configuration and persistent data
- Use explicit version tags for reproducibility
- Limit container privileges for security
- Monitor logs and admin API for service health

For more details, see the user and development documentation.
