# Stacker Development & Technical Documentation

## Architecture
- Written in Go, designed for concurrency and reliability
- Core: Process supervision engine, service manager, admin API

## Process Supervision API
The `Process` interface provides methods for starting, stopping, and monitoring processes, as well as subscribing to process events:

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
 }
```
- Event listeners for stdout, stderr, and exit
- Goroutine and channel-based concurrency
- Graceful shutdown and cleanup

## Configuration Loading
- Supports YAML and JSON
- Environment variable substitution
- OS-specific config search paths

## Service Management
- Service selection, optional/required services
- Restart policies (always, on-failure, exponential, etc.)
- Cron job support

## Admin API
- REST endpoints for service control
- WebSocket log streaming
- Example JSON responses

## Testing & CI
- Unit and integration tests
- GitHub Actions for CI

## Contribution
- PRs welcome! See code style and test guidelines in this document.
