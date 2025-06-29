package supervisor

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/adhocore/gronx"
	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/log"
)

type cronService struct {
	name      string
	schedule  string
	processes []Process
	enabled   bool
	single    bool
	grace     time.Duration
	command   []string
	workDir   string
	env       map[string]string

	stdoutListeners []chan string
	stderrListeners []chan string

	scheduler *gronx.Gronx
	verbose   bool

	mu sync.Mutex

	forwardingStopChans []chan struct{}
	ctx                 context.Context
}

// NewCronService creates a new CronService from a config.ServiceEntry
func NewCronService(config *config.ServiceEntry, scheduler *gronx.Gronx) CronService {
	if config == nil {
		return nil
	}

	// Validate cron expression
	if config.Cron == "" {
		return nil
	}

	if !scheduler.IsValid(config.Cron) {
		return nil
	}

	ctx := context.Background()
	// Create the service
	service := &cronService{
		name:                "", // Will be set by the caller
		schedule:            config.Cron,
		processes:           []Process{},
		enabled:             true, // Default to enabled
		single:              config.Single,
		grace:               config.GracePeriod,
		command:             config.Command,
		workDir:             config.WorkingDir,
		env:                 config.Env,
		scheduler:           scheduler,
		verbose:             true, // Default to verbose logging
		forwardingStopChans: []chan struct{}{},
		ctx:                 ctx,
	}

	return service
}

func (c *cronService) GetName() string {
	return c.name
}

func (c *cronService) GetType() ServiceType {
	return ServiceTypeCron
}

func (c *cronService) AsProcess() ProcessService {
	return nil
}

func (c *cronService) AsCron() CronService {
	return c
}

func (c *cronService) IsSingle() bool {
	return c.single
}

func (c *cronService) IsEnabled() bool {
	return c.enabled
}

func (c *cronService) GetSchedule() string {
	return c.schedule
}

func (c *cronService) SetEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = enabled
}

// GetSerializedStats returns a map of serialized statistics for the cron service
func (c *cronService) GetSerializedStats() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()

	stats := map[string]any{
		"type":     "cron",
		"name":     c.name,
		"schedule": c.schedule,
		"enabled":  c.enabled,
		"single":   c.single,
		"running":  len(c.processes) > 0,
		"count":    len(c.processes),
	}

	return stats
}

// StopAll stops all processes in the cron service
func (c *cronService) StopAll() error {
	c.mu.Lock()
	processes := append([]Process{}, c.processes...)
	forwardingStopChans := append([]chan struct{}{}, c.forwardingStopChans...)
	c.processes = []Process{}
	c.forwardingStopChans = nil
	c.mu.Unlock()

	for _, process := range processes {
		if process.IsRunning() {
			if err := process.Stop(c.grace); err != nil {
				_ = process.Kill()
			}
			if sp, ok := process.(*supervisedProcess); ok {
				select {
				case <-sp.done:
					// exited
				case <-time.After(5 * time.Second):
					log.Errorf("supervisor", "cron job %s process did not exit after kill", c.name)
				}
			}
		}
	}

	for _, stopChan := range forwardingStopChans {
		close(stopChan)
	}

	log.Printf("supervisor", "[%s] All processes and output forwarders stopped", c.name)
	return nil
}

// GetCommand returns the command to run
func (c *cronService) GetCommand() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.command
}

// GetWorkDir returns the working directory
func (c *cronService) GetWorkDir() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.workDir
}

// GetEnv returns the environment variables
func (c *cronService) GetEnv() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.env
}

// GetProcesses returns the processes
func (c *cronService) GetProcesses() []Process {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.processes
}

func (c *cronService) OnStdout(listener chan string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Prevent adding duplicate listeners
	if slices.Contains(c.stdoutListeners, listener) {
		return
	}
	c.stdoutListeners = append(c.stdoutListeners, listener)
}

func (c *cronService) OnStderr(listener chan string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if slices.Contains(c.stderrListeners, listener) {
		return
	}
	c.stderrListeners = append(c.stderrListeners, listener)
}

func (c *cronService) OffStdout(listener chan string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, ch := range c.stdoutListeners {
		if ch == listener {
			c.stdoutListeners = append(c.stdoutListeners[:i], c.stdoutListeners[i+1:]...)
			break
		}
	}
}

func (c *cronService) OffStderr(listener chan string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, ch := range c.stderrListeners {
		if ch == listener {
			c.stderrListeners = append(c.stderrListeners[:i], c.stderrListeners[i+1:]...)
			break
		}
	}
}

// addProcess adds a process to the cron service and sets up exit handling
func (c *cronService) addProcess(process Process) {
	c.mu.Lock()
	c.processes = append(c.processes, process)
	name := c.name
	verbose := c.verbose
	c.mu.Unlock()

	// Attach a single handler to process output
	process.OnStdout(makeCronServiceOutputHandler(c, name, "STDOUT"))
	process.OnStderr(makeCronServiceOutputHandler(c, name, "STDERR"))

	// Attach exit handler to log exit code and remove process from slice
	exitCh := make(chan int, 1)
	process.OnExit(exitCh)
	go func(proc Process, exitCh chan int) {
		exitCode := <-exitCh
		if verbose {
			log.Printf("supervisor", "cron job %s process exited with code: %d", name, exitCode)
		}
		// Remove process from slice
		c.mu.Lock()
		for i, p := range c.processes {
			if p == proc {
				c.processes = append(c.processes[:i], c.processes[i+1:]...)
				break
			}
		}
		c.mu.Unlock()
	}(process, exitCh)
}

func makeCronServiceOutputHandler(c *cronService, name, stream string) chan string {
	ch := make(chan string, 100)
	go func() {
		for line := range ch {
			log.Printf("supervisor", "[%s %s] %s", name, stream, line)
			c.mu.Lock()
			var listeners []chan string
			if stream == "STDOUT" {
				listeners = append([]chan string{}, c.stdoutListeners...)
			} else {
				listeners = append([]chan string{}, c.stderrListeners...)
			}
			c.mu.Unlock()
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

// Run checks if the cron job should run at time t and starts the process if due
func (c *cronService) Run(t time.Time) {
	// Lock only to check state and copy command/env
	c.mu.Lock()
	enabled := c.enabled
	single := c.single
	processCount := len(c.processes)
	workDir := c.workDir
	env := map[string]string{}
	maps.Copy(env, c.env)
	verbose := c.verbose
	schedule := c.schedule
	scheduler := c.scheduler
	name := c.name
	c.mu.Unlock()

	if !enabled {
		return
	}

	isDue, err := scheduler.IsDue(schedule, t)
	if err != nil {
		if verbose {
			log.Errorf("supervisor", "error checking cron schedule for %s: %s", name, err)
		}
		return
	}

	if isDue {

		if single && processCount > 0 {
			if verbose {
				log.Printf("supervisor", "skipping cron job %s (already running)", name)
			}
			return
		}

		command := append([]string{}, c.command...)

		cfg := ProcessConfig{
			Path:    command[0],
			Args:    command[1:],
			WorkDir: workDir,
			Env:     env,
		}
		
		process := NewProcess(c.ctx, cfg)
		c.addProcess(process)
		if err := process.Start(); err != nil {
			log.Errorf("supervisor", "failed to start cron job %s: %s", name, err)
			return
		}
		log.Printf("supervisor", "cron job %s process started, pid: %v, cmd: %v", name, process.GetPid(), command)
	}
}
