# Stacker Development & Technical Documentation

## Architecture
- Written in Go, designed for concurrency and reliability
- Core: Process supervision engine, service manager, admin API

## Process Supervision API
The `Process` interface provides methods for starting, stopping, and monitoring processes, as well as subscribing to process events. Each process is assigned a unique ID (using nanoid) and supports resource monitoring:

```go
 type Process interface {
     Start() error
     Stop(timeout time.Duration) error
     Kill() error
     IsRunning() bool
     OnStdout(listener chan string)
     OnStderr(listener chan string)
     OnExit(listener chan int)
     OffStdout(listener chan string)
     OffStderr(listener chan string)
     OffExit(listener chan int)
     GetID() string // Unique process ID
     GetResourceStats() (*ProcessResourceStats, error) // CPU/memory usage
 }

 type ProcessResourceStats struct {
     CPUPercent    float64
     MemoryRSS     uint64
     MemoryPercent float32
 }
```
- Each process is assigned a unique ID for tracking and API access
- Resource stats are available for each process (cross-platform)

## Service & Process API
- Services expose all processes under them via `ListProcesses()` and allow lookup by ID with `GetProcessByID(id)`
- The admin API allows querying and log streaming for both services and individual processes

## Configuration Loading
- Supports YAML and JSON
- Environment variable substitution
- OS-specific config search paths

## Service Management
- Service selection, optional/required services
- Restart policies (always, on-failure, exponential, etc.)
- Cron job support

## Admin API
- REST endpoints for service and process control
- WebSocket log streaming for both services and individual processes
- Example endpoints:
  - `/process/{id}`: Get process status and resource usage
  - `/ws/process/{id}/logs`: Stream logs for a specific process

## Testing & CI
- Unit and integration tests
- GitHub Actions for CI

## Contribution
- PRs welcome! See code style and test guidelines in this document.
