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
- Unique Process IDs: Each process is assigned a unique ID for precise management and monitoring
- HTTP Admin Interface: RESTful API, real-time status, log streaming, filterable output
- Process-level API: Query and stream logs for individual processes
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

### Service Endpoints
- `/services`: List all services and their status
- `/service/{name}`: Get status for a specific service
- `/service/{name}/start|stop|restart`: Control a service
- `/service/{name}/processes`: List all processes under a specific service
- `/ws/service/{name}/logs`: Stream logs for a service (all processes)

### Process-level Endpoints
- `/processes`: List all processes across all services
- `/process/{id}`: Get status and resource usage for a specific process (by unique process ID)
- `/ws/process/{id}/logs`: Stream logs for a specific process

### Service Resource Usage Endpoint
- `/service/{name}/resource`: Get the total (summed) CPU and memory usage for all processes under a service

#### Example: Get process status
```sh
curl http://localhost:8080/process/abc123
```
Response:
```json
{
  "id": "abc123",
  "pid": 12345,
  "path": "/usr/bin/myapp",
  "args": ["--foo"],
  "workDir": "/srv/app",
  "env": {"FOO": "bar"},
  "running": true,
  "resource": {
    "cpu_percent": 0.5,
    "memory_rss": 12345678,
    "memory_percent": 0.1
  }
}
```

#### Example: Stream logs for a process
Connect to `ws://localhost:8080/ws/process/{id}/logs` using a WebSocket client.

#### Example: Get resource usage for a service
```sh
curl http://localhost:8080/service/nginx/resource
```
Response:
```json
{
  "cpu_percent": 1.2,
  "memory_rss": 23456789,
  "memory_percent": 0.3
}
```

#### Example: List all processes
```sh
curl http://localhost:8080/processes
```
Response:
```json
[
  {
    "id": "abc123",
    "pid": 12345,
    "service": "nginx",
    "running": true,
    "resource": { "cpu_percent": 0.5, "memory_rss": 12345678, "memory_percent": 0.1 }
  },
  // ...
]
```

#### Example: List processes under a service
```sh
curl http://localhost:8080/service/nginx/processes
```
Response:
```json
[
  {
    "id": "abc123",
    "pid": 12345,
    "running": true,
    "resource": { "cpu_percent": 0.5, "memory_rss": 12345678, "memory_percent": 0.1 }
  }
]
```

### TypeScript API Types

Below are TypeScript type definitions for the main admin interface endpoints:

```ts
// Resource usage for a process or service
export interface ResourceStats {
  cpu_percent: number;
  memory_rss: number;
  memory_percent: number;
  process_count?: number; // Only present for service resource endpoint
}

// Process status response
export interface ProcessStatus {
  id: string;
  pid: number;
  path: string;
  args: string[];
  workDir: string;
  env: Record<string, string>;
  running: boolean;
  resource: ResourceStats;
}

// Service resource usage response
export interface ServiceResourceResponse {
  resource: ResourceStats;
}

// Log message from WebSocket endpoints
export interface LogMessage {
  stream: 'stdout' | 'stderr';
  line: string;
}

// Example: /process/{id}
// const status: ProcessStatus = await fetch(...)

// Example: /service/{name}/resource
// const usage: ServiceResourceResponse = await fetch(...)

// Example: WebSocket log usage
// const msg: LogMessage = JSON.parse(receivedData)
// msg.stream === 'stdout' // or 'stderr'
// msg.line // log line content
```

## Testing
- Run tests: `go test ./...`
- Coverage, race detector, and CI info

For more details, see the technical documentation in [docs/development.md](development.md).
