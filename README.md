# Stacker

[![Go Report Card](https://goreportcard.com/badge/github.com/eslym/stacker)](https://goreportcard.com/report/github.com/eslym/stacker)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/eslym/stacker)](https://github.com/eslym/stacker)

Stacker is a robust process supervisor designed for containerized environments. It efficiently manages the lifecycle of multiple services, providing advanced monitoring, automatic restart capabilities, and a comprehensive admin interface.

> [!NOTE]
> This application is fully written by AI (JetBrains Junie).

## 📋 Features

- **Service Management**: Start, stop, and monitor multiple services from a single control point
- **Flexible Restart Policies**: Configure with delays, exponential backoff, and conditional restart options
- **Cron Scheduling**: Run periodic tasks with standard cron syntax
- **Conflict Resolution**: Automatically manage services that cannot run simultaneously
- **Optional Services**: Selectively include or exclude services at runtime
- **Resource Monitoring**: Track CPU and memory usage of managed processes
- **HTTP Admin Interface**:
  - RESTful API for service control
  - Real-time service status monitoring
  - Live log streaming via Server-Sent Events (SSE)
  - Filterable log output
- **Configuration Flexibility**: Support for both YAML and JSON configuration formats

## 🚀 Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/eslym/stacker.git

# Build the application
cd stacker
go build -o stacker

# Optional: Install to system path
sudo mv stacker /usr/local/bin/
```

### Using Go Install

```bash
go install github.com/eslym/stacker@latest
```

### Using Docker

```bash
# Pull the latest image (Debian-based)
docker pull eslym/stacker:latest

# Run with default configuration
docker run -v /path/to/config:/usr/local/etc/stacker -p 8080:8080 eslym/stacker
```

## 🐳 Docker Usage

Stacker provides official Docker images on Docker Hub that support both Debian (glibc) and Alpine (musl) distributions, as well as both x64 (amd64) and arm64 architectures.

### Available Tags

- `eslym/stacker:latest` - Latest stable release (Debian-based)
- `eslym/stacker:x.y.z` - Specific version (e.g., `1.0.0`) (Debian-based)
- `eslym/stacker:x.y` - Latest patch version of a specific minor release (e.g., `1.0`) (Debian-based)
- `eslym/stacker:x` - Latest minor and patch version of a specific major release (e.g., `1`) (Debian-based)
- `eslym/stacker:x.y.z-debian` - Specific version (Debian-based, explicit)
- `eslym/stacker:x.y-debian` - Latest patch version (Debian-based, explicit)
- `eslym/stacker:x-debian` - Latest minor and patch version (Debian-based, explicit)
- `eslym/stacker:latest-alpine` - Latest stable release (Alpine-based)
- `eslym/stacker:x.y.z-alpine` - Specific version on Alpine (e.g., `1.0.0-alpine`)
- `eslym/stacker:x.y-alpine` - Latest patch version on Alpine (e.g., `1.0-alpine`)
- `eslym/stacker:x-alpine` - Latest minor and patch version on Alpine (e.g., `1-alpine`)

### Basic Usage

```bash
# Run with default configuration
docker run -v /path/to/config:/usr/local/etc/stacker eslym/stacker

# Run with a specific configuration file
docker run -v /path/to/config.yaml:/config.yaml eslym/stacker --config /config.yaml

# Expose admin interface
docker run -v /path/to/config:/usr/local/etc/stacker -p 8080:8080 eslym/stacker
```

### Running Specific Services

```bash
# Run only specific services
docker run eslym/stacker nginx php-fpm

# Run with optional services
docker run eslym/stacker --with=horizon

# Run all services except specific ones
docker run eslym/stacker --except scheduler
```

### Docker Compose Example

```yaml
version: '3'

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

### Building Custom Images

You can build custom images based on the official Stacker image:

```Dockerfile
FROM eslym/stacker:latest

# Add your configuration
COPY config.yaml /usr/local/etc/stacker/config.yaml

# Set default command
CMD ["/usr/local/bin/stacker", "--verbose"]
```

## ⚙️ Configuration

Stacker uses a configuration file in YAML or JSON format. By default, it looks for a configuration file at:
- Windows: `C:\ProgramData\Stacker\config.yaml`
- Linux: `/usr/local/etc/stacker/config.yaml`

You can specify a different configuration file using the `--config` flag.

### Example Configuration (YAML)

Below is an example configuration for a containerized Laravel application:

```yaml
# Global settings
restart: 5s
grace: 5s
admin:
  host: 0.0.0.0
  port: 8080

# Service definitions
services:
  # Web server
  nginx:
    cmd: ["nginx", "-g", "daemon off;"]
    restart: true
    env:
      NGINX_HOST: "app.local"
      NGINX_PORT: "80"

  # PHP-FPM service
  php-fpm:
    cmd: ["php-fpm", "--nodaemonize"]
    restart: true
    cwd: /var/www/html

  # Laravel application server
  laravel:
    cmd: ["php", "artisan", "serve", "--host=0.0.0.0", "--port=8000"]
    cwd: /var/www/html
    restart: 
      mode: "on-failure"
      exponential: true
      max: "1m"

  # Scheduled tasks
  scheduler:
    cmd: ["php", "artisan", "schedule:run"]
    cwd: /var/www/html
    cron: "* * * * *"

  # Queue worker
  queue:
    cmd: ["php", "artisan", "queue:work", "--tries=3"]
    cwd: /var/www/html
    restart: true
    conflict: horizon
    optional: true

  # Laravel Horizon (alternative queue processor)
  horizon:
    cmd: ["php", "artisan", "horizon"]
    cwd: /var/www/html
    restart: true
    conflict: queue
    optional: true
```

### Configuration Options

#### Global Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `restart` | Default restart base time for all processes | `5s` |
| `grace` | Default grace period for all processes | `5s` |
| `admin` | HTTP admin interface configuration | `disabled` |

#### Admin Interface Configuration

| Option | Description |
|--------|-------------|
| `host` and `port` | Host and port for the HTTP server |
| `sock` | Unix socket path (Linux only) |

#### Service Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `cmd` | Command to run (string or array of strings) | *Required* |
| `cwd` | Working directory for the process | Current directory |
| `env` | Environment variables for the process | Inherit from parent |
| `grace` | Grace period for this process | Global `grace` value |
| `optional` | Whether this process is optional | `false` |
| `conflict` | Other services which will conflict with this process | `[]` |
| `restart` | Restart policy for this process | `true` |
| `cron` | Cron schedule for this process (mutually exclusive with restart) | None |
| `single` | Whether this process is a single instance | `false` |

#### Restart Policy Options

| Value | Description |
|-------|-------------|
| `true` | Always restart (shorthand for `{ mode: 'always' }`) |
| `false` | Never restart |
| `"exponential"` | Restart with exponential backoff |
| `"immediate"` | Restart immediately (no delay) |
| Object | Custom restart configuration (see below) |

##### Custom Restart Configuration

| Option | Description | Default |
|--------|-------------|---------|
| `mode` | `"always"`, `"never"`, or `"on-failure"` | *Required* |
| `base` | Base time for restart | Global `restart` value |
| `exponential` | Whether to use exponential backoff | `false` |
| `max` | Maximum time for restart | `1h` |
| `maxRetries` | Maximum retries | Infinity |

## 🖥️ Usage

> [!WARNING]
> Usage on Windows is very experimental. Some process handling features are not available on Windows, which may affect functionality and stability.

### Basic Usage

```bash
# Run all services (except optional ones)
stacker

# Run all services including nginx, php-fpm, laravel, scheduler, and horizon
stacker --with=horizon

# Run only nginx and php-fpm services
stacker nginx php-fpm

# Run all services except scheduler
stacker --except scheduler --with=queue
```

### Command-Line Options

| Option | Shorthand | Description |
|--------|-----------|-------------|
| `--config` | `-c` | Path to the configuration file |
| `--with` | `-w` | Comma-separated list of optional services to include |
| `--without` | `-x` | Comma-separated list of services to exclude |
| `--except` | `-e` | Treat services as blacklist (optional services will not run) |
| `--help` | `-h` | Show help message |

## 🔧 Admin Interface

If enabled in the configuration, the admin interface provides HTTP endpoints for monitoring and controlling services.

### Endpoints

#### Service Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/services` | GET | Get status of all services |
| `/api/services/{name}` | GET | Get status of a specific service |
| `/api/services/{name}` | POST | Control a service |

For the POST endpoint, the request body should be:
```json
{
  "action": "start|stop|restart"
}
```

Service actions are handled differently based on service type:
- **Regular services**:
  - `start`: Start the service
  - `stop`: Stop the service
  - `restart`: Restart the service
- **Cron jobs**:
  - `start`: Enable the cron job
  - `stop`: Disable the cron job
  - `restart`: Not applicable (returns an error)

#### Log Streaming

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/logs` | GET | Stream logs via Server-Sent Events (SSE) |

Query parameters:
- `service`: Filter logs by service name

### Service Status Response

The service status response includes detailed information about the service:

```json
{
  "name": "laravel",
  "status": "running",
  "pid": 1234,
  "startTime": "2023-07-01T12:00:00Z",
  "uptime": "1h30m45s",
  "restartCount": 2,
  "resourceUsage": {
    "cpuPercent": 5.2,
    "memoryUsage": "64.5 MB",
    "lastUpdated": "2023-07-01T13:30:45Z"
  },
  "actions": {
    "start": {
      "description": "Start the service",
      "applicable": false
    },
    "stop": {
      "description": "Stop the service",
      "applicable": true
    },
    "restart": {
      "description": "Restart the service",
      "applicable": true
    }
  },
  "isCronJob": false
}
```

## 🧪 Testing

Stacker includes a comprehensive test suite to ensure reliability and correctness. The tests cover all major components of the application:

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with race detector
go test -race ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test Coverage

The test suite includes:

- **Unit Tests**: Testing individual components in isolation
  - Configuration loading and validation
  - Command-line argument parsing
  - Service selection logic
  - Supervisor lifecycle management
  - Admin interface endpoints

- **Integration Tests**: Testing the interaction between components
  - Admin interface with supervisor
  - Service management through the admin API

### Continuous Integration

Stacker uses GitHub Actions for continuous integration. Every push and pull request triggers the test suite to ensure code quality and prevent regressions.

## 📄 License

MIT
