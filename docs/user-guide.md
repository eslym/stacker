# Stacker User Guide

## Overview
Stacker is a robust process supervisor designed for containerized environments. It efficiently manages the lifecycle of multiple services, providing advanced monitoring, automatic restart capabilities, and a comprehensive admin interface.

## Features
- Service Management: Start, stop, and monitor multiple services from a single control point
- Flexible Restart Policies: Configure with delays, exponential backoff, and conditional restart options
- Cron Scheduling: Run periodic tasks with standard cron syntax
- Conflict Resolution: Automatically manage services that cannot run simultaneously
- Optional Services: Selectively include or exclude services at runtime
- Resource Monitoring: Track CPU and memory usage of managed processes
- HTTP Admin Interface: RESTful API, real-time status, log streaming, filterable output
- Configuration Flexibility: YAML/JSON

## Installation
### From Source
```bash
git clone https://github.com/eslym/stacker.git
cd stacker
go build -o stacker
sudo mv stacker /usr/local/bin/
```
### Using Go Install
```bash
go install github.com/eslym/stacker@latest
```
### Using Docker
```bash
docker pull eslym/stacker:latest
docker run -v /path/to/config:/usr/local/etc/stacker -p 8080:8080 eslym/stacker
```

For Docker-specific usage, see [docs/docker.md](docker.md).

## Configuration
- YAML or JSON
- Search order: STACKER_CONFIG_PATH, current dir, user config dir, OS-specific defaults
- Example config and options documented in [docs/configuration.md](configuration.md)

### Example Configuration (YAML)
```yaml
# Global settings
delay: 5s
grace: 30s
admin:
  host: 0.0.0.0
  port: 8080
services:
  nginx:
    cmd: ["nginx", "-g", "daemon off;"]
    restart: true
    env:
      NGINX_HOST: "app.local"
      NGINX_PORT: "80"
  php-fpm:
    cmd: ["php-fpm", "--nodaemonize"]
    restart: true
    workDir: /var/www/html
  laravel:
    cmd: ["php", "artisan", "serve", "--host=0.0.0.0", "--port=8000"]
    workDir: /var/www/html
    restart:
      mode: "on-failure"
      exponential: true
      delay: 10s
      maxDelay: 1m
      maxRetries: 5
  scheduler:
    cmd: ["php", "artisan", "schedule:run"]
    workDir: /var/www/html
    cron: "* * * * *"
  queue:
    cmd: ["php", "artisan", "queue:work", "--tries=3"]
    workDir: /var/www/html
    restart: true
    optional: true
  horizon:
    cmd: ["php", "artisan", "horizon"]
    workDir: /var/www/html
    restart: true
    optional: true
```

### Configuration Options
- See [docs/development.md](development.md) for full option reference and technical details.

## Usage
- CLI flags: --config, --with, --except, --help, etc.
- Service selection and optional services

### Command-Line Options
| Option      | Shorthand | Description                                                  |
| ----------- | --------- | ------------------------------------------------------------ |
| `--config`  | `-c`      | Path to the configuration file                               |
| `--with`    | `-w`      | Comma-separated list of optional services to include         |
| `--without` | `-x`      | Comma-separated list of services to exclude                  |
| `--except`  | `-e`      | Treat services as blacklist (optional services will not run) |
| `--help`    | `-h`      | Show help message                                            |

## Admin Interface
- HTTP endpoints for service management and log streaming
- Example API endpoints and responses

## Testing
- Run tests: `go test ./...`
- Coverage, race detector, and CI info

For more details, see the technical documentation in [docs/development.md](development.md).
