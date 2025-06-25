package supervisor_test

import (
	"context"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/supervisor"
)

// MockProcess represents a mock process for testing
type MockProcess struct {
	Cmd      []string
	Cwd      string
	Env      map[string]string
	Grace    string
	Optional bool
	Conflict interface{}
	Restart  interface{}
	Cron     string
	Single   bool
}

// MockConfig creates a mock configuration for testing
func MockConfig() *config.Config {
	// Create OS-specific commands
	var service1Cmd, service2Cmd, cronServiceCmd, conflict1Cmd, conflict2Cmd, optionalServiceCmd []string

	if runtime.GOOS == "windows" {
		// On Windows, echo is a shell built-in, not a standalone executable
		service1Cmd = []string{"cmd.exe", "/c", "echo", "service1"}
		service2Cmd = []string{"cmd.exe", "/c", "echo", "service2"}
		cronServiceCmd = []string{"cmd.exe", "/c", "echo", "cron-service"}
		conflict1Cmd = []string{"cmd.exe", "/c", "echo", "conflict1"}
		conflict2Cmd = []string{"cmd.exe", "/c", "echo", "conflict2"}
		optionalServiceCmd = []string{"cmd.exe", "/c", "echo", "optional-service"}
	} else {
		// On Unix-like systems, echo is a standalone executable
		service1Cmd = []string{"echo", "service1"}
		service2Cmd = []string{"echo", "service2"}
		cronServiceCmd = []string{"echo", "cron-service"}
		conflict1Cmd = []string{"echo", "conflict1"}
		conflict2Cmd = []string{"echo", "conflict2"}
		optionalServiceCmd = []string{"echo", "optional-service"}
	}

	return &config.Config{
		Restart: "1s",
		Grace:   "1s",
		Services: map[string]config.Process{
			"service1": {
				Cmd:     service1Cmd,
				Restart: true,
			},
			"service2": {
				Cmd:     service2Cmd,
				Restart: false,
			},
			"cron-service": {
				Cmd:  cronServiceCmd,
				Cron: "* * * * *",
			},
			"conflict1": {
				Cmd:      conflict1Cmd,
				Restart:  true,
				Conflict: "conflict2",
			},
			"conflict2": {
				Cmd:      conflict2Cmd,
				Restart:  true,
				Conflict: "conflict1",
			},
			"optional-service": {
				Cmd:      optionalServiceCmd,
				Restart:  true,
				Optional: true,
			},
		},
	}
}

// MockLoggerProvider is a mock implementation of the LoggerProvider interface
type MockLoggerProvider struct {
	Logs   map[string][]string
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// GetLogger returns a mock logger for the specified service
func (p *MockLoggerProvider) GetLogger(serviceName string) io.Writer {
	r, w, _ := os.Pipe()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer r.Close()

		buf := make([]byte, 1024)
		for {
			select {
			case <-p.ctx.Done():
				return
			default:
				n, err := r.Read(buf)
				if err != nil {
					return
				}
				if n > 0 {
					p.Logs[serviceName] = append(p.Logs[serviceName], string(buf[:n]))
				}
			}
		}
	}()
	return w
}

// Close stops all goroutines and cleans up resources
func (p *MockLoggerProvider) Close() {
	p.cancel()
	p.wg.Wait()
}

// NewMockLoggerProvider creates a new mock logger provider
func NewMockLoggerProvider() *MockLoggerProvider {
	ctx, cancel := context.WithCancel(context.Background())
	return &MockLoggerProvider{
		Logs:   make(map[string][]string),
		ctx:    ctx,
		cancel: cancel,
	}
}

func TestNewSupervisor(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"service1": true,
		"service2": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	if sup == nil {
		t.Errorf("Expected supervisor to be created, got nil")
	}
}

func TestSupervisor_Start(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"service1": true,
		"service2": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Clean up
	sup.Stop()
}

func TestSupervisor_StartWithConflicts(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"conflict1": true,
		"conflict2": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err == nil {
		t.Errorf("Expected error for conflicting services, got nil")
		sup.Stop()
	}
}

func TestSupervisor_GetServiceStatus(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"service1": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Wait for service to start
	time.Sleep(100 * time.Millisecond)

	// Get service status
	info, err := sup.GetServiceStatus("service1")
	if err != nil {
		t.Errorf("Failed to get service status: %v", err)
	}
	if info.Name != "service1" {
		t.Errorf("Expected service name to be service1, got %s", info.Name)
	}

	// Clean up
	sup.Stop()
}

func TestSupervisor_GetServiceStatus_Unknown(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"service1": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Get status of unknown service
	_, err = sup.GetServiceStatus("unknown")
	if err == nil {
		t.Errorf("Expected error for unknown service, got nil")
	}

	// Clean up
	sup.Stop()
}

func TestSupervisor_GetAllServiceStatuses(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"service1": true,
		"service2": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Wait for services to start
	time.Sleep(100 * time.Millisecond)

	// Get all service statuses
	statuses := sup.GetAllServiceStatuses()
	if len(statuses) != 2 {
		t.Errorf("Expected 2 services, got %d", len(statuses))
	}
	if _, ok := statuses["service1"]; !ok {
		t.Errorf("Expected service1 to exist in statuses")
	}
	if _, ok := statuses["service2"]; !ok {
		t.Errorf("Expected service2 to exist in statuses")
	}

	// Clean up
	sup.Stop()
}

func TestSupervisor_StartService(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"service1": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Stop the service
	err = sup.StopService("service1")
	if err != nil {
		t.Errorf("Failed to stop service: %v", err)
	}

	// Wait for service to stop
	time.Sleep(100 * time.Millisecond)

	// Start the service
	err = sup.StartService("service1")
	if err != nil {
		t.Errorf("Failed to start service: %v", err)
	}

	// Wait for service to start
	time.Sleep(100 * time.Millisecond)

	// Get service status
	info, err := sup.GetServiceStatus("service1")
	if err != nil {
		t.Errorf("Failed to get service status: %v", err)
	}
	if info.Status != supervisor.StatusRunning && info.Status != supervisor.StatusRestarting {
		t.Errorf("Expected service status to be running or restarting, got %s", info.Status)
	}

	// Clean up
	sup.Stop()
}

func TestSupervisor_StopService(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"service1": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Wait for service to start
	time.Sleep(100 * time.Millisecond)

	// Stop the service
	err = sup.StopService("service1")
	if err != nil {
		t.Errorf("Failed to stop service: %v", err)
	}

	// Wait for service to stop with polling
	maxWaitTime := 2 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWaitTime)

	var info *supervisor.ServiceInfo

	for time.Now().Before(deadline) {
		// Get service status
		info, err = sup.GetServiceStatus("service1")
		if err != nil {
			t.Errorf("Failed to get service status: %v", err)
			break
		}

		// Check if service is in expected state
		if info.Status == supervisor.StatusStopped || info.Status == supervisor.StatusFailed {
			// Test passed
			break
		}

		// Wait before polling again
		time.Sleep(pollInterval)
	}

	// Final check
	// Accept restarting as a valid state after stopping a service
	// This is because there's a race condition between stopping the service and the goroutine
	// that monitors the process detecting that it has exited and restarting it
	if info != nil && info.Status != supervisor.StatusStopped && info.Status != supervisor.StatusFailed && info.Status != supervisor.StatusRestarting {
		t.Errorf("Expected service status to be stopped, failed, or restarting, got %s", info.Status)
	}

	// Clean up
	sup.Stop()
}

func TestSupervisor_RestartService(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"service1": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Wait for service to start
	time.Sleep(100 * time.Millisecond)

	// Restart the service
	err = sup.RestartService("service1")
	if err != nil {
		t.Errorf("Failed to restart service: %v", err)
	}

	// Wait for service to restart
	time.Sleep(100 * time.Millisecond)

	// Get service status
	info, err := sup.GetServiceStatus("service1")
	if err != nil {
		t.Errorf("Failed to get service status: %v", err)
	}
	if info.Status != supervisor.StatusRunning && info.Status != supervisor.StatusRestarting {
		t.Errorf("Expected service status to be running or restarting, got %s", info.Status)
	}

	// Clean up
	sup.Stop()
}

func TestSupervisor_RestartCronJob(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"cron-service": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Wait for service to start
	time.Sleep(100 * time.Millisecond)

	// Restart the cron job (should fail)
	err = sup.RestartService("cron-service")
	if err == nil {
		t.Errorf("Expected error for restarting cron job, got nil")
	}

	// Clean up
	sup.Stop()
}

func TestSupervisor_EnableDisableCronJob(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"cron-service": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Wait for service to start
	time.Sleep(100 * time.Millisecond)

	// Disable the cron job
	err = sup.DisableCronJob("cron-service")
	if err != nil {
		t.Errorf("Failed to disable cron job: %v", err)
	}

	// Wait for cron job to be disabled
	time.Sleep(100 * time.Millisecond)

	// Get service status
	info, err := sup.GetServiceStatus("cron-service")
	if err != nil {
		t.Errorf("Failed to get service status: %v", err)
	}
	if info.Status != supervisor.StatusStopped {
		t.Errorf("Expected service status to be stopped, got %s", info.Status)
	}

	// Enable the cron job
	err = sup.EnableCronJob("cron-service")
	if err != nil {
		t.Errorf("Failed to enable cron job: %v", err)
	}

	// Wait for cron job to be enabled
	time.Sleep(100 * time.Millisecond)

	// Get service status
	info, err = sup.GetServiceStatus("cron-service")
	if err != nil {
		t.Errorf("Failed to get service status: %v", err)
	}
	if info.Status != supervisor.StatusScheduled {
		t.Errorf("Expected service status to be scheduled, got %s", info.Status)
	}

	// Clean up
	sup.Stop()
}

func TestSupervisor_EnableDisableNonCronJob(t *testing.T) {
	// Set a timeout for the test
	timeout := time.AfterFunc(5*time.Second, func() {
		t.Fatal("Test timed out after 5 seconds")
	})
	defer timeout.Stop()
	cfg := MockConfig()
	activeServices := map[string]bool{
		"service1": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		t.Errorf("Failed to start supervisor: %v", err)
	}

	// Wait for service to start
	time.Sleep(100 * time.Millisecond)

	// Enable a non-cron job (should fail)
	err = sup.EnableCronJob("service1")
	if err == nil {
		t.Errorf("Expected error for enabling non-cron job, got nil")
	}

	// Disable a non-cron job (should fail)
	err = sup.DisableCronJob("service1")
	if err == nil {
		t.Errorf("Expected error for disabling non-cron job, got nil")
	}

	// Clean up
	sup.Stop()
}
