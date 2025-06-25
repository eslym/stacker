package supervisor

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/eslym/stacker/pkg/config"
	"github.com/robfig/cron/v3"
)

// ServiceStatus represents the status of a service
type ServiceStatus string

const (
	StatusStopped    ServiceStatus = "stopped"
	StatusRunning    ServiceStatus = "running"
	StatusFailed     ServiceStatus = "failed"
	StatusRestarting ServiceStatus = "restarting"
	StatusScheduled  ServiceStatus = "scheduled"
)

// ServiceInfo represents information about a service
type ServiceInfo struct {
	Name         string
	Status       ServiceStatus
	Pid          int
	Uptime       time.Duration
	StartTime    time.Time
	RestartCount int
	NextRestart  time.Time
	NextRun      time.Time
	ExitCode     int
	Error        string
	Cmd          *exec.Cmd
	Process      *os.Process
	Config       config.Process
	// Resource usage
	CpuPercent  float64
	MemoryUsage int64
	LastUpdated time.Time
}

// LoggerProvider is an interface for providing loggers for services
type LoggerProvider interface {
	GetLogger(serviceName string) io.Writer
}

// Supervisor manages the lifecycle of services
type Supervisor struct {
	config         *config.Config
	services       map[string]*ServiceInfo
	activeServices map[string]bool
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	cron           *cron.Cron
	cronEntries    map[string]cron.EntryID
	loggerProvider LoggerProvider
	verbose        bool
}

// NewSupervisor creates a new supervisor
func NewSupervisor(cfg *config.Config, activeServices map[string]bool, verbose bool) *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())

	sup := &Supervisor{
		config:         cfg,
		services:       make(map[string]*ServiceInfo),
		activeServices: activeServices,
		ctx:            ctx,
		cancel:         cancel,
		cron:           cron.New(),
		cronEntries:    make(map[string]cron.EntryID),
		verbose:        verbose,
	}

	// Start resource usage monitoring
	go sup.monitorResourceUsage()

	return sup
}

// SetLoggerProvider sets the logger provider for the supervisor
func (s *Supervisor) SetLoggerProvider(provider LoggerProvider) {
	s.loggerProvider = provider
}

// monitorResourceUsage periodically updates resource usage information for running services
func (s *Supervisor) monitorResourceUsage() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.updateResourceUsage()
		}
	}
}

// updateResourceUsage updates resource usage information for all running services
func (s *Supervisor) updateResourceUsage() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, info := range s.services {
		if info.Status == StatusRunning && info.Process != nil {
			// On Windows, we would use the Windows Management Instrumentation (WMI) API
			// to get process information. For simplicity, we'll just use random values.

			// Simulate CPU usage (0-100%)
			info.CpuPercent = float64(rand.Intn(100))

			// Simulate memory usage (0-1GB)
			info.MemoryUsage = int64(rand.Intn(1024 * 1024 * 1024))

			info.LastUpdated = time.Now()
		}
	}
}

// Start starts the supervisor
func (s *Supervisor) Start() error {
	// Initialize services
	servicesToStart := make([]string, 0)
	cronJobsToSchedule := make([]string, 0)
	var initErr error

	// Initialize services with lock
	s.mu.Lock()

	// Initialize services
	for name, proc := range s.config.Services {
		if !s.activeServices[name] {
			if s.verbose {
				log.Printf("Skipping inactive service: %s", name)
			}
			continue
		}

		// Check for conflicts
		if conflicts := getConflicts(proc); len(conflicts) > 0 {
			if s.verbose {
				log.Printf("Checking conflicts for service %s: %v", name, conflicts)
			}
			for _, conflict := range conflicts {
				if s.activeServices[conflict] {
					initErr = fmt.Errorf("service %s conflicts with %s", name, conflict)
					s.mu.Unlock()
					return initErr
				}
			}
		}

		if s.verbose {
			log.Printf("Initializing service: %s", name)
		}
		s.services[name] = &ServiceInfo{
			Name:   name,
			Status: StatusStopped,
			Config: proc,
		}
	}

	// Check if there are any services to run
	if len(s.services) == 0 {
		s.mu.Unlock()
		return fmt.Errorf("no services to run")
	}

	// Collect services to start and cron jobs to schedule
	for name, info := range s.services {
		if info.Config.Cron != "" {
			cronJobsToSchedule = append(cronJobsToSchedule, name)
		} else {
			servicesToStart = append(servicesToStart, name)
		}
	}

	// Release the lock before scheduling cron jobs and starting services
	s.mu.Unlock()

	// Schedule cron jobs
	if s.verbose {
		log.Printf("Scheduling %d cron jobs", len(cronJobsToSchedule))
	}
	for _, name := range cronJobsToSchedule {
		s.scheduleCronJob(name)
	}

	// Start services
	if s.verbose {
		log.Printf("Starting %d services", len(servicesToStart))
	}
	for _, name := range servicesToStart {
		if err := s.startService(name); err != nil {
			log.Printf("Failed to start service %s: %v", name, err)
		}
	}

	// Start cron scheduler
	if s.verbose {
		log.Printf("Starting cron scheduler")
	}
	s.cron.Start()

	return nil
}

// Stop stops the supervisor
func (s *Supervisor) Stop() {
	if s.verbose {
		log.Printf("Stopping supervisor")
	}
	s.cancel()

	// Stop cron scheduler
	if s.verbose {
		log.Printf("Stopping cron scheduler")
	}
	ctx := s.cron.Stop()

	// Collect services to stop
	var serviceNames []string

	s.mu.Lock()
	for name := range s.services {
		serviceNames = append(serviceNames, name)
	}
	s.mu.Unlock()

	if s.verbose {
		log.Printf("Stopping %d services", len(serviceNames))
	}

	// Stop all services without holding the lock
	for _, name := range serviceNames {
		if err := s.stopService(name); err != nil {
			log.Printf("Failed to stop service %s: %v", name, err)
		}
	}

	// Wait for all services to stop
	if s.verbose {
		log.Printf("Waiting for all services to stop")
	}
	s.wg.Wait()

	// Wait for cron jobs to finish
	if s.verbose {
		log.Printf("Waiting for cron jobs to finish")
	}
	<-ctx.Done()

	if s.verbose {
		log.Printf("Supervisor stopped successfully")
	}
}

// startService starts a service
func (s *Supervisor) startService(name string) error {
	// Get service info
	var info *ServiceInfo
	var exists bool

	if s.verbose {
		log.Printf("Attempting to start service: %s", name)
	}

	// Check if service exists
	func() {
		s.mu.RLock()
		defer s.mu.RUnlock()
		info, exists = s.services[name]
	}()

	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	// Check if service is already running
	if info.Status == StatusRunning {
		if s.verbose {
			log.Printf("Service %s is already running", name)
		}
		return nil
	}

	// Acquire lock for the rest of the operation
	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-check if service exists and is not running after acquiring the lock
	info, exists = s.services[name]
	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	if info.Status == StatusRunning {
		return nil
	}

	// Prepare command
	var cmdArgs []string
	switch cmd := info.Config.Cmd.(type) {
	case string:
		cmdParts := strings.Fields(cmd)
		cmdArgs = append(cmdArgs, cmdParts...)
	case []interface{}:
		for _, arg := range cmd {
			if strArg, ok := arg.(string); ok {
				cmdArgs = append(cmdArgs, strArg)
			}
		}
	}

	if len(cmdArgs) == 0 {
		return fmt.Errorf("invalid command for service %s", name)
	}

	// Create command
	command := exec.CommandContext(s.ctx, cmdArgs[0], cmdArgs[1:]...)

	// Set working directory
	if info.Config.Cwd != "" {
		command.Dir = info.Config.Cwd
	}

	// Set environment variables
	if len(info.Config.Env) > 0 {
		command.Env = os.Environ()
		for k, v := range info.Config.Env {
			command.Env = append(command.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Set up output redirection if logger provider is available
	if s.loggerProvider != nil {
		logger := s.loggerProvider.GetLogger(name)
		command.Stdout = logger
		command.Stderr = logger
	}

	// Start the process
	if s.verbose {
		log.Printf("Starting process for service %s: %s %v", name, cmdArgs[0], cmdArgs[1:])
	}

	if err := command.Start(); err != nil {
		info.Status = StatusFailed
		info.Error = err.Error()
		if s.verbose {
			log.Printf("Failed to start service %s: %v", name, err)
		}
		return err
	}

	// Update service info
	info.Cmd = command
	info.Process = command.Process
	info.Pid = command.Process.Pid
	info.Status = StatusRunning
	info.StartTime = time.Now()
	info.Error = ""

	if s.verbose {
		log.Printf("Service %s started successfully with PID %d", name, info.Pid)
	}

	// Monitor the process
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// Wait for the process to exit
		err := command.Wait()

		s.mu.Lock()
		defer s.mu.Unlock()

		// Update service info
		info.Status = StatusStopped
		info.Uptime = time.Since(info.StartTime)
		info.Process = nil

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				info.ExitCode = exitErr.ExitCode()
			}
			info.Error = err.Error()
			info.Status = StatusFailed
		}

		// Handle restart policy
		if s.shouldRestart(info) {
			delay := s.calculateRestartDelay(info)
			info.Status = StatusRestarting
			info.NextRestart = time.Now().Add(delay)

			time.AfterFunc(delay, func() {
				if err := s.startService(name); err != nil {
					log.Printf("Failed to restart service %s: %v", name, err)
				} else {
					info.RestartCount++
				}
			})
		}
	}()

	return nil
}

// stopService stops a service
func (s *Supervisor) stopService(name string) error {
	// Get service info
	var info *ServiceInfo
	var exists bool
	var process *os.Process

	if s.verbose {
		log.Printf("Attempting to stop service: %s", name)
	}

	// Check if service exists and is running
	func() {
		s.mu.RLock()
		defer s.mu.RUnlock()
		info, exists = s.services[name]
		if exists && info.Status == StatusRunning {
			process = info.Process
		}
	}()

	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	if info.Status != StatusRunning || process == nil {
		if s.verbose {
			log.Printf("Service %s is not running, nothing to stop", name)
		}
		return nil
	}

	// Get grace period from config
	var graceDuration time.Duration
	func() {
		s.mu.RLock()
		defer s.mu.RUnlock()

		grace := s.config.Grace
		if info.Config.Grace != "" {
			grace = info.Config.Grace
		}

		// Parse grace period
		var err error
		graceDuration, err = time.ParseDuration(grace)
		if err != nil {
			graceDuration = 5 * time.Second
		}
	}()

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), graceDuration)
	defer cancel()

	// Send SIGTERM
	if s.verbose {
		log.Printf("Sending SIGTERM to service %s (PID %d) with grace period %v", name, process.Pid, graceDuration)
	}

	if err := process.Signal(os.Interrupt); err != nil {
		log.Printf("Failed to send SIGTERM to service %s: %v", name, err)
		// Force kill
		if s.verbose {
			log.Printf("Force killing service %s (PID %d)", name, process.Pid)
		}
		if err := process.Kill(); err != nil {
			return err
		}
	}

	// Wait for the process to exit or timeout
	select {
	case <-ctx.Done():
		// Force kill
		if s.verbose {
			log.Printf("Grace period expired for service %s, force killing (PID %d)", name, process.Pid)
		}
		if err := process.Kill(); err != nil {
			return err
		}
	default:
		// Process exited gracefully
		if s.verbose {
			log.Printf("Service %s exited gracefully", name)
		}
	}

	// Update service status
	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-check if service exists
	info, exists = s.services[name]
	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	info.Status = StatusStopped
	return nil
}

// scheduleCronJob schedules a cron job
func (s *Supervisor) scheduleCronJob(name string) {
	// Get service info
	var info *ServiceInfo
	var schedule string
	var exists bool

	if s.verbose {
		log.Printf("Scheduling cron job for service: %s", name)
	}

	// Check if service exists and get cron schedule
	s.mu.RLock()
	info, exists = s.services[name]
	if exists {
		schedule = info.Config.Cron
	}
	s.mu.RUnlock()

	if !exists {
		log.Printf("Service %s not found", name)
		return
	}

	// Check if cron schedule is specified
	if schedule == "" {
		log.Printf("No cron schedule specified for service %s", name)
		return
	}

	if s.verbose {
		log.Printf("Cron schedule for service %s: %s", name, schedule)
	}

	// Update service status
	s.mu.Lock()
	// Re-check if service exists
	info, exists = s.services[name]
	if exists {
		info.Status = StatusScheduled
	}
	s.mu.Unlock()

	// Get command and configuration
	var cmdType interface{}
	var cmdArgs []string
	var cwd string
	var env map[string]string

	// Get command configuration with read lock
	s.mu.RLock()
	// Re-check if service exists
	info, exists = s.services[name]
	if exists {
		cmdType = info.Config.Cmd
		cwd = info.Config.Cwd
		env = info.Config.Env
	}
	s.mu.RUnlock()

	// Schedule the job
	entryID, err := s.cron.AddFunc(schedule, func() {
		// Use timestamp in log message
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		log.Printf("[%s] [%s] Running scheduled job", timestamp, name)

		// Create a new command for each run
		cmdArgs = []string{}
		switch cmd := cmdType.(type) {
		case string:
			cmdParts := strings.Fields(cmd)
			cmdArgs = append(cmdArgs, cmdParts...)
		case []interface{}:
			for _, arg := range cmd {
				if strArg, ok := arg.(string); ok {
					cmdArgs = append(cmdArgs, strArg)
				}
			}
		}

		if len(cmdArgs) == 0 {
			log.Printf("Invalid command for service %s", name)
			return
		}

		// Create command
		command := exec.Command(cmdArgs[0], cmdArgs[1:]...)

		// Set working directory
		if cwd != "" {
			command.Dir = cwd
		}

		// Set environment variables
		if len(env) > 0 {
			command.Env = os.Environ()
			for k, v := range env {
				command.Env = append(command.Env, fmt.Sprintf("%s=%v", k, v))
			}
		}

		// Get logger if available
		var logger io.Writer
		s.mu.RLock()
		if s.loggerProvider != nil {
			logger = s.loggerProvider.GetLogger(name)
		}
		s.mu.RUnlock()

		// Update service info before command execution
		startTime := time.Now()
		s.mu.Lock()
		// Re-check if service exists
		var serviceInfo *ServiceInfo
		serviceExists := false
		serviceInfo, serviceExists = s.services[name]
		if serviceExists {
			serviceInfo.Status = StatusRunning
			serviceInfo.StartTime = startTime
			serviceInfo.NextRun = time.Time{}
		}
		s.mu.Unlock()

		// Set up output redirection if logger provider is available
		var output []byte
		var err error
		if logger != nil {
			command.Stdout = logger
			command.Stderr = logger
			err = command.Run()
		} else {
			output, err = command.CombinedOutput()
		}

		// Update service info after command execution
		s.mu.Lock()
		// Re-check if service exists
		serviceInfo, serviceExists = s.services[name]
		if serviceExists {
			serviceInfo.Status = StatusScheduled
			serviceInfo.Uptime = time.Since(startTime)

			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					serviceInfo.ExitCode = exitErr.ExitCode()
				}
				serviceInfo.Error = err.Error()
				// Use timestamp in log message
				timestamp := time.Now().Format("2006-01-02 15:04:05")
				log.Printf("[%s] [%s] Scheduled job failed: %v", timestamp, name, err)
			} else {
				serviceInfo.ExitCode = 0
				serviceInfo.Error = ""
			}

			// Calculate next run time
			if entryID, ok := s.cronEntries[name]; ok {
				if entry := s.cron.Entry(entryID); !entry.Next.IsZero() {
					serviceInfo.NextRun = entry.Next
				}
			}
		}
		s.mu.Unlock()

		// Log output
		if len(output) > 0 {
			// Use timestamp in log message
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			log.Printf("[%s] [%s] Output from scheduled job: %s", timestamp, name, string(output))
		}
	})

	if err != nil {
		// Use timestamp in log message
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		log.Printf("[%s] [%s] Failed to schedule job: %v", timestamp, name, err)
		return
	}

	// Store entry ID and calculate next run time
	s.mu.Lock()
	// Re-check if service exists
	info, exists = s.services[name]
	if exists {
		// Store entry ID
		s.cronEntries[name] = entryID

		// Calculate next run time
		if entry := s.cron.Entry(entryID); !entry.Next.IsZero() {
			info.NextRun = entry.Next
		}
	}
	s.mu.Unlock()
}

// shouldRestart determines if a service should be restarted
func (s *Supervisor) shouldRestart(info *ServiceInfo) bool {
	if info.Config.Cron != "" {
		return false
	}

	restart := info.Config.Restart
	if restart == nil {
		return false
	}

	switch r := restart.(type) {
	case bool:
		return r
	case string:
		return r == "true" || r == "always" || r == "exponential" || r == "immediate"
	case map[string]interface{}:
		if mode, ok := r["mode"].(string); ok {
			return mode == "always" || mode == "on-failure" && info.ExitCode != 0
		}
	}

	return false
}

// calculateRestartDelay calculates the delay before restarting a service
func (s *Supervisor) calculateRestartDelay(info *ServiceInfo) time.Duration {
	// Default restart delay
	baseDelay := "5s"
	if s.config.Restart != "" {
		baseDelay = s.config.Restart
	}

	// Get restart policy
	restart := info.Config.Restart

	// Parse base delay
	var delay time.Duration
	var err error

	switch r := restart.(type) {
	case string:
		if r == "immediate" {
			return 0
		}
		delay, err = time.ParseDuration(baseDelay)
	case map[string]interface{}:
		if base, ok := r["base"].(string); ok {
			delay, err = time.ParseDuration(base)
		} else {
			delay, err = time.ParseDuration(baseDelay)
		}

		// Apply exponential backoff
		if exp, ok := r["exponential"].(bool); ok && exp {
			delay = delay * time.Duration(info.RestartCount+1)
		}

		// Apply maximum delay
		if max, ok := r["max"].(string); ok {
			maxDelay, maxErr := time.ParseDuration(max)
			if maxErr == nil && delay > maxDelay {
				delay = maxDelay
			}
		}
	default:
		delay, err = time.ParseDuration(baseDelay)
	}

	if err != nil {
		delay = 5 * time.Second
	}

	return delay
}

// GetServiceStatus returns the status of a service
func (s *Supervisor) GetServiceStatus(name string) (*ServiceInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, exists := s.services[name]
	if !exists {
		return nil, fmt.Errorf("service %s not found", name)
	}

	return info, nil
}

// GetAllServiceStatuses returns the status of all services
func (s *Supervisor) GetAllServiceStatuses() map[string]*ServiceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*ServiceInfo)
	for name, info := range s.services {
		result[name] = info
	}

	return result
}

// StartService starts a service
func (s *Supervisor) StartService(name string) error {
	return s.startService(name)
}

// StopService stops a service
func (s *Supervisor) StopService(name string) error {
	return s.stopService(name)
}

// RestartService restarts a service
func (s *Supervisor) RestartService(name string) error {
	info, err := s.GetServiceStatus(name)
	if err != nil {
		return err
	}

	// If it's a cron job, restart is not applicable
	if info.Config.Cron != "" {
		return fmt.Errorf("restart is not applicable for cron job %s", name)
	}

	if err := s.stopService(name); err != nil {
		return err
	}

	return s.startService(name)
}

// EnableCronJob enables a cron job
func (s *Supervisor) EnableCronJob(name string) error {
	info, err := s.GetServiceStatus(name)
	if err != nil {
		return err
	}

	// Check if it's a cron job
	if info.Config.Cron == "" {
		return fmt.Errorf("service %s is not a cron job", name)
	}

	// If already scheduled, do nothing
	if info.Status == StatusScheduled {
		return nil
	}

	// Schedule the cron job
	s.scheduleCronJob(name)
	return nil
}

// DisableCronJob disables a cron job
func (s *Supervisor) DisableCronJob(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.services[name]
	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	// Check if it's a cron job
	if info.Config.Cron == "" {
		return fmt.Errorf("service %s is not a cron job", name)
	}

	// If already stopped, do nothing
	if info.Status == StatusStopped {
		return nil
	}

	// Remove the cron entry
	entryID, exists := s.cronEntries[name]
	if exists {
		s.cron.Remove(entryID)
		delete(s.cronEntries, name)
	}

	// Update service info
	info.Status = StatusStopped
	info.NextRun = time.Time{}

	return nil
}

// getConflicts returns the list of services that conflict with a service
func getConflicts(proc config.Process) []string {
	var conflicts []string

	switch c := proc.Conflict.(type) {
	case string:
		if c != "" {
			conflicts = append(conflicts, c)
		}
	case []interface{}:
		for _, conflict := range c {
			if strConflict, ok := conflict.(string); ok && strConflict != "" {
				conflicts = append(conflicts, strConflict)
			}
		}
	}

	return conflicts
}
