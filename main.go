package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/eslym/stacker/pkg/admin"
	"github.com/eslym/stacker/pkg/cli"
	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/supervisor"
	"gopkg.in/yaml.v3"
)

// ServiceLoggerProvider implements the supervisor.LoggerProvider interface
type ServiceLoggerProvider struct {
	adminServer *admin.AdminServer
}

// GetLogger returns a logger for the specified service
func (p *ServiceLoggerProvider) GetLogger(serviceName string) io.Writer {
	return NewServiceLogger(serviceName, p.adminServer)
}

// ServiceLogger is a custom logger that prepends timestamp and service name to each line of output
type ServiceLogger struct {
	serviceName string
	adminServer *admin.AdminServer
}

// Write implements io.Writer interface
func (l *ServiceLogger) Write(p []byte) (n int, err error) {
	// Split output by lines
	lines := strings.Split(string(p), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Prepend timestamp and service name
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		formattedLine := fmt.Sprintf("[%s] [%s] %s", timestamp, l.serviceName, line)

		// Write to stdout
		fmt.Println(formattedLine)

		// Send to admin interface if available
		if l.adminServer != nil {
			l.adminServer.BroadcastLog(l.serviceName, line)
		}
	}

	return len(p), nil
}

// NewServiceLogger creates a new service logger
func NewServiceLogger(serviceName string, adminServer *admin.AdminServer) *ServiceLogger {
	return &ServiceLogger{
		serviceName: serviceName,
		adminServer: adminServer,
	}
}

func main() {
	fmt.Println("Stacker - Service Supervisor")

	// Parse command-line arguments
	options, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("Error parsing arguments: %v", err)
	}

	// Load configuration
	cfg, err := config.LoadConfig(options.ConfigPath)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Log the normalized config if verbose flag is enabled
	if options.Verbose {
		fmt.Println("Normalized configuration:")
		// Create a normalized config with inherited values explicitly set
		normalizedCfg := config.NormalizeConfig(cfg)
		if cfgYaml, err := yaml.Marshal(normalizedCfg); err == nil {
			fmt.Println(string(cfgYaml))
		} else {
			log.Printf("Error marshaling config to YAML: %v", err)
		}
	}

	// Determine which services to run
	availableServices := make(map[string]bool)
	optionalServices := make(map[string]bool)
	for name, service := range cfg.Services {
		availableServices[name] = true
		if service.Optional {
			optionalServices[name] = true
		}
	}

	activeServices, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err != nil {
		log.Fatalf("Error selecting services: %v", err)
	}

	// Initialize supervisor
	sup := supervisor.NewSupervisor(cfg, activeServices, options.Verbose)

	// Initialize admin interface
	adminServer := admin.NewAdminServer(cfg, sup)

	// Create logger provider
	loggerProvider := &ServiceLoggerProvider{
		adminServer: adminServer,
	}

	// Set logger provider on supervisor
	sup.SetLoggerProvider(loggerProvider)

	// Start supervisor
	if err := sup.Start(); err != nil {
		log.Fatalf("Error starting supervisor: %v", err)
	}

	// Start admin interface
	if err := adminServer.Start(); err != nil {
		log.Printf("Error starting admin server: %v", err)
	}

	// Use timestamp in log message
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] [stacker] Stacker started successfully\n", timestamp)

	// Handle signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	<-sigCh
	// Use timestamp in log message
	shutdownTimestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] [stacker] Shutting down...\n", shutdownTimestamp)

	// Create a channel to signal when shutdown is complete
	shutdownComplete := make(chan struct{})

	// Start the shutdown process in a goroutine
	go func() {
		// Stop admin interface
		if err := adminServer.Stop(); err != nil {
			log.Printf("Error stopping admin server: %v", err)
		}

		// Stop supervisor
		sup.Stop()

		close(shutdownComplete)
	}()

	// Wait for shutdown to complete with a timeout
	select {
	case <-shutdownComplete:
		// Shutdown completed normally
	case <-time.After(60 * time.Second):
		log.Printf("WARNING: Shutdown timed out after 60 seconds, forcing exit")
	}

	// Use timestamp in log message
	completeTimestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] [stacker] Shutdown complete\n", completeTimestamp)
}
