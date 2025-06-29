package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/log"
)

var ErrFatal = errors.New("fatal error")

const (
	restartNoRestart = iota
	restartScheduled
	restartUserStopped
	restartRetryExceed
)

const minRestartDelay = 50 * time.Millisecond

type processService struct {
	name          string
	restartPolicy RestartPolicy
	retries       int
	lastExitCode  int
	process       Process
	userStopped   bool
	grace         time.Duration

	stdoutListeners []chan string
	stderrListeners []chan string

	verbose bool

	ctx context.Context
	mu  sync.Mutex
}

// NewProcessService creates a new ProcessService from a config.ServiceEntry
func NewProcessService(config *config.ServiceEntry) ProcessService {
	if config == nil {
		return nil
	}

	// Create the service struct first to get the context
	service := &processService{
		name:            "", // Will be set by the caller
		restartPolicy:   NewRestartPolicy(config.Restart),
		retries:         0,
		lastExitCode:    0,
		userStopped:     false,
		grace:           config.GracePeriod,
		verbose:         true, // Default to verbose logging
		ctx:             context.Background(),
		stdoutListeners: []chan string{},
		stderrListeners: []chan string{},
	}
	// Now create the process with the service context
	cfg := ProcessConfig{
		Path:    config.Command[0],
		Args:    config.Command[1:],
		WorkDir: config.WorkingDir,
		Env:     config.Env,
	}
	service.process = NewProcess(service.ctx, cfg)

	// Attach a single handler to process output
	service.process.OnStdout(makeProcessServiceOutputHandler(service, "STDOUT"))
	service.process.OnStderr(makeProcessServiceOutputHandler(service, "STDERR"))

	go service.watchForExit()

	return service
}

func makeProcessServiceOutputHandler(service *processService, stream string) chan string {
	ch := make(chan string, 100)
	go func() {
		for line := range ch {
			log.Printf("supervisor", "[%s %s] %s", service.name, stream, line)
			service.mu.Lock()
			var listeners []chan string
			if stream == "STDOUT" {
				listeners = append([]chan string{}, service.stdoutListeners...)
			} else {
				listeners = append([]chan string{}, service.stderrListeners...)
			}
			service.mu.Unlock()
			for _, l := range listeners {
				select {
				case l <- line:
				default:
				}
			}
		}
	}()
	return ch
}

func (p *processService) GetName() string {
	return p.name
}

func (p *processService) GetType() ServiceType {
	return ServiceTypeProcess
}

func (p *processService) AsProcess() ProcessService {
	return p
}

func (p *processService) AsCron() CronService {
	return nil
}

func (p *processService) IsRunning() bool {
	return p.process.IsRunning()
}

func (p *processService) GetRetries() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.retries
}

func (p *processService) GetRestartPolicy() RestartPolicy {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.restartPolicy
}

func (p *processService) GetLastExitCode() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastExitCode
}

func (p *processService) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process.IsRunning() {
		return fmt.Errorf("%w", ErrProcessRunning)
	}
	p.retries = 0
	p.userStopped = false
	if p.process.IsRunning() {
		return fmt.Errorf("%w", ErrProcessRunning)
	}
	err := p.process.Start()
	if err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}
	if p.verbose {
		log.Printf(
			"supervisor", "service %s process started, pid: %d, cmd: %s %v",
			p.name, p.process.GetPid(), p.process.GetPath(), p.process.GetArgs(),
		)
	}
	return nil
}

func (p *processService) Stop(user bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.process.IsRunning() {
		return fmt.Errorf("%w", ErrProcessNotRunning)
	}
	return p.stop(user)
}

func (p *processService) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.process.IsRunning() {
		return fmt.Errorf("%w", ErrProcessNotRunning)
	}
	err := p.process.Kill()
	if err != nil {
		if errors.Is(err, ErrProcessNotRunning) {
			return err
		} else {
			return fmt.Errorf("%w: failed to kill process: %w", ErrFatal, err)
		}
	}
	return nil
}

func (p *processService) Restart() error {
	p.mu.Lock()

	// If the process is running, try to stop it
	var stopErr error
	if p.process.IsRunning() {
		stopErr = p.stop(true)
		if stopErr != nil && !errors.Is(stopErr, ErrProcessNotRunning) {
			p.mu.Unlock()
			return stopErr
		}
		// Wait for process to fully stop (up to 1 second)
		for i := 0; i < 20 && p.process.IsRunning(); i++ {
			p.mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			p.mu.Lock()
		}
		if p.process.IsRunning() {
			// As a last resort, forcibly clean up process state
			if sp, ok := p.process.(*supervisedProcess); ok {
				sp.ForceCleanup()
			}
			p.mu.Unlock()
			return fmt.Errorf("cannot restart: process still running after stop, forced cleanup performed")
		}
	}

	// Release the lock before calling Start to avoid deadlock
	p.mu.Unlock()

	// Just call Start, do not recreate the Process object
	return p.Start()
}

func (p *processService) stop(user bool) error {
	if p.process == nil {
		return fmt.Errorf("%w", ErrProcessNotRunning)
	}
	p.userStopped = user
	err := p.process.Stop(p.grace)

	if err == nil {
		return nil
	}

	if errors.Is(err, ErrStopProcessTimeout) && p.verbose {
		log.Errorf("supervisor", "process %s did not stop within %s, killing it: %s", p.name, p.grace, err)
	}

	if err := p.process.Kill(); err != nil {
		return fmt.Errorf("%w: failed to kill process: %w", ErrFatal, err)
	}
	return nil
}

func (p *processService) nextRestart(success bool) (int, time.Duration) {
	if p.restartPolicy.GetMode() == RestartModeNever || p.restartPolicy.GetMode() == RestartModeOnFailure && success {
		return restartNoRestart, time.Duration(0)
	}
	if p.userStopped {
		return restartUserStopped, time.Duration(0)
	}
	if p.restartPolicy.GetMaxRetries() > 0 && p.retries >= p.restartPolicy.GetMaxRetries() {
		return restartRetryExceed, time.Duration(0)
	}
	if !success {
		p.retries++
	}
	delay := p.restartPolicy.GetInitialDelay()
	if p.restartPolicy.IsExponential() {
		delay *= time.Duration(1 << p.retries)
	}
	if p.restartPolicy.GetMaxDelay() > 0 && delay > p.restartPolicy.GetMaxDelay() {
		delay = p.restartPolicy.GetMaxDelay()
	}
	if delay < minRestartDelay {
		delay = minRestartDelay
	}
	return restartScheduled, delay
}

// GetSerializedStats returns a map of serialized statistics for the process service
func (p *processService) GetSerializedStats() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := map[string]any{
		"type":        "process",
		"name":        p.name,
		"running":     p.process.IsRunning(),
		"retries":     p.retries,
		"exitCode":    p.lastExitCode,
		"userStopped": p.userStopped,
	}

	// Add restart policy information
	if p.restartPolicy != nil {
		stats["restart"] = map[string]any{
			"mode":         p.restartPolicy.GetMode().String(),
			"exponential":  p.restartPolicy.IsExponential(),
			"maxRetries":   p.restartPolicy.GetMaxRetries(),
			"initialDelay": p.restartPolicy.GetInitialDelay().String(),
			"maxDelay":     p.restartPolicy.GetMaxDelay().String(),
		}
	}

	return stats
}

func (p *processService) watchForExit() {
	listener := make(chan int)
	p.process.OnExit(listener)
	for {
		select {
		case <-p.ctx.Done():
			return
		case exitCode, ok := <-listener:
			if !ok {
				return
			}
			p.mu.Lock()
			p.lastExitCode = exitCode
			p.mu.Unlock()
			restart, delay := p.nextRestart(exitCode == 0)
			switch restart {
			case restartScheduled:
				if p.verbose {
					log.Printf("supervisor", "process %s exied with code %d, will restart in %s", p.name, exitCode, delay)
				}
				go p.restartAfter(delay)
			case restartUserStopped:
				if p.verbose {
					log.Printf("supervisor", "process %s exied with code %d, stopped by user not restarting", p.name, exitCode)
				}
			case restartRetryExceed:
				if p.verbose {
					log.Printf("supervisor", "process %s exied with code %d, exceeded max retries (%d), not restarting", p.name, exitCode, p.restartPolicy.GetMaxRetries())
				}
			}
		}
	}
}

func (p *processService) restartAfter(delay time.Duration) {
	timer := time.NewTimer(delay)
	select {
	case <-timer.C:
		err := p.Start()
		if err != nil {
			log.Errorf("supervisor", "failed to restart process %s after delay: %s", p.name, err)
		}
	case <-p.ctx.Done():
		timer.Stop()
	}
}

func (p *processService) OffStdout(listener chan string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, ch := range p.stdoutListeners {
		if ch == listener {
			p.stdoutListeners = append(p.stdoutListeners[:i], p.stdoutListeners[i+1:]...)
			break
		}
	}
}

func (p *processService) OffStderr(listener chan string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, ch := range p.stderrListeners {
		if ch == listener {
			p.stderrListeners = append(p.stderrListeners[:i], p.stderrListeners[i+1:]...)
			break
		}
	}
}

func (p *processService) OnStdout(listener chan string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, l := range p.stdoutListeners {
		if l == listener {
			return
		}
	}
	p.stdoutListeners = append(p.stdoutListeners, listener)
}

func (p *processService) OnStderr(listener chan string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, l := range p.stderrListeners {
		if l == listener {
			return
		}
	}
	p.stderrListeners = append(p.stderrListeners, listener)
}
