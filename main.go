package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/adhocore/gronx"
	"github.com/eslym/stacker/pkg/admin"
	"github.com/eslym/stacker/pkg/cli"
	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/log"
	"github.com/eslym/stacker/pkg/supervisor"
	"gopkg.in/yaml.v3"
)

func main() {
	// Parse command-line arguments
	options, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		log.Errorf("main", "Error: %s", err)
		os.Exit(1)
	}

	// Resolve config path
	configPath := options.ConfigPath
	if configPath == "" {
		configPath, err = config.ResolveConfigPath("stacker.json", "stacker.yaml", "stacker.yml")
		if err != nil {
			log.Errorf("main", "Error: %s", err)
			os.Exit(1)
		}
	}

	// Load config file
	configData, err := os.ReadFile(configPath)
	if err != nil {
		log.Errorf("main", "Error reading config file: %s", err)
		os.Exit(1)
	}

	// Parse config file
	var data any
	if len(configData) > 0 {
		ext := strings.ToLower(filepath.Ext(configPath))
		if ext == ".yaml" || ext == ".yml" {
			if err := yaml.Unmarshal(configData, &data); err != nil {
				log.Errorf("main", "Error parsing YAML config: %s", err)
				os.Exit(1)
			}
		} else {
			if err := json.Unmarshal(configData, &data); err != nil {
				log.Errorf("main", "Error parsing JSON config: %s", err)
				os.Exit(1)
			}
		}
	}

	// Create cron parser
	gron := gronx.New()

	configDir := filepath.Dir(configPath)

	// Deserialize config
	cfg, err := config.DeserializeConfig(data, func(env string) string {
		if env == "configDir" {
			return configDir
		}
		return os.Getenv(env)
	}, gron)
	if err != nil {
		log.Errorf("main", "Error deserializing config: %s", err)
		os.Exit(1)
	}

	if options.Verbose {
		var buffer bytes.Buffer
		encoder := yaml.NewEncoder(&buffer)
		encoder.SetIndent(2)
		if err := encoder.Encode(cfg.Serialize()); err != nil {
			log.Errorf("main", "Error encoding config to YAML: %s", err)
			os.Exit(1)
		}
		_ = encoder.Close()
		log.Printf("main", "Loaded configuration from %s\n%s", configPath, buffer.String())
	}

	// Get available services
	availableServices := make(map[string]bool)
	optionalServices := make(map[string]bool)
	for name, entry := range cfg.Services {
		availableServices[name] = true
		if entry.Optional {
			optionalServices[name] = true
		}
	}

	// Determine which services to run
	serviceSelection, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err != nil {
		log.Errorf("main", "Error: %s", err)
		os.Exit(1)
	}

	if len(serviceSelection) == 0 {
		log.Errorf("main", "No services selected to run. Please specify services using --service or --all.")
		os.Exit(1)
	}

	if options.Verbose {
		log.Printf("main", "Selected services: %v", serviceSelection)
	}

	// Create supervisor
	sup, err := supervisor.CreateSupervisorFromConfig(cfg, serviceSelection, options.Verbose)
	if err != nil {
		log.Errorf("main", "Error creating supervisor: %s", err)
		os.Exit(1)
	}

	// Start supervisor
	if err := sup.Start(); err != nil {
		log.Errorf("main", "Error starting supervisor: %s", err)
		os.Exit(1)
	}

	// Start admin HTTP interface if enabled
	var adminServer interface{ Stop() error }
	if cfg.Admin != nil {
		// Use a background context for admin, but you may want to use supervisor's context if available
		adminSrv := admin.NewAdminServer(context.Background(), sup, cfg.Admin)
		if err := adminSrv.Start(); err != nil {
			log.Errorf("main", "Error starting admin HTTP server: %s", err)
			os.Exit(1)
		}
		adminServer = adminSrv
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	sig := <-sigChan
	log.Printf("main", "Received signal %s, shutting down...", sig)

	// Stop supervisor
	if err := sup.Stop(); err != nil {
		log.Errorf("main", "Error stopping supervisor: %s", err)
		os.Exit(1)
	}

	// Stop admin HTTP server if running
	if adminServer != nil {
		_ = adminServer.Stop()
	}

	log.Printf("main", "Shutdown complete")
}
