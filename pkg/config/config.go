package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RestartPolicy defines how a process should be restarted
type RestartPolicy struct {
	Mode        string `json:"mode" yaml:"mode"`
	Base        string `json:"base,omitempty" yaml:"base,omitempty"`
	Exponential bool   `json:"exponential,omitempty" yaml:"exponential,omitempty"`
	Max         string `json:"max,omitempty" yaml:"max,omitempty"`
	MaxRetries  int    `json:"maxRetries,omitempty" yaml:"maxRetries,omitempty"`
}

// Process defines a service process configuration
type Process struct {
	Cmd      interface{}       `json:"cmd" yaml:"cmd"`
	Cwd      string            `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Env      map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Grace    string            `json:"grace,omitempty" yaml:"grace,omitempty"`
	Optional bool              `json:"optional,omitempty" yaml:"optional,omitempty"`
	Conflict interface{}       `json:"conflict,omitempty" yaml:"conflict,omitempty"`
	Restart  interface{}       `json:"restart,omitempty" yaml:"restart,omitempty"`
	Cron     string            `json:"cron,omitempty" yaml:"cron,omitempty"`
	Single   bool              `json:"single,omitempty" yaml:"single,omitempty"`
}

// AdminConfig defines the HTTP admin interface configuration
type AdminConfig struct {
	Host string `json:"host,omitempty" yaml:"host,omitempty"`
	Port int    `json:"port,omitempty" yaml:"port,omitempty"`
	Sock string `json:"sock,omitempty" yaml:"sock,omitempty"`
}

// Config defines the main application configuration
type Config struct {
	Restart  string             `json:"restart,omitempty" yaml:"restart,omitempty"`
	Grace    string             `json:"grace,omitempty" yaml:"grace,omitempty"`
	Admin    interface{}        `json:"admin,omitempty" yaml:"admin,omitempty"`
	Services map[string]Process `json:"services" yaml:"services"`
}

// GetDefaultConfigPath returns the default config path based on the OS
func GetDefaultConfigPath() string {
	// Windows default path
	return "C:\\ProgramData\\Stacker\\config.yaml"
}

// LoadConfig loads the configuration from a file
func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = GetDefaultConfigPath()
	}

	// Check if the file exists
	_, err := os.Stat(configPath)
	if err != nil {
		return nil, fmt.Errorf("config file not found: %s", configPath)
	}

	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %s", err)
	}

	var config Config

	// Determine file type by extension
	ext := strings.ToLower(filepath.Ext(configPath))
	switch ext {
	case ".json":
		err = json.Unmarshal(data, &config)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &config)
	default:
		return nil, errors.New("unsupported config file format, use .json or .yaml")
	}

	if err != nil {
		return nil, fmt.Errorf("error parsing config file: %s", err)
	}

	// Validate the config
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	if len(config.Services) == 0 {
		return errors.New("no services defined in config")
	}

	// Set defaults
	if config.Restart == "" {
		config.Restart = "5s"
	}

	if config.Grace == "" {
		config.Grace = "5s"
	}

	// Validate services
	for name, service := range config.Services {
		// Validate command
		switch cmd := service.Cmd.(type) {
		case string:
			if cmd == "" {
				return fmt.Errorf("empty command for service %s", name)
			}
		case []interface{}:
			if len(cmd) == 0 {
				return fmt.Errorf("empty command array for service %s", name)
			}
		default:
			return fmt.Errorf("invalid command type for service %s", name)
		}

		// Validate restart policy and cron (mutually exclusive)
		if service.Restart != nil && service.Cron != "" {
			return fmt.Errorf("service %s has both restart policy and cron schedule, which are mutually exclusive", name)
		}
	}

	return nil
}

// NormalizeConfig creates a copy of the config with all inherited values explicitly set
func NormalizeConfig(config *Config) *Config {
	// Create a deep copy of the config
	normalizedConfig := &Config{
		Restart:  config.Restart,
		Grace:    config.Grace,
		Admin:    config.Admin,
		Services: make(map[string]Process),
	}

	// Copy services with normalized values
	for name, service := range config.Services {
		normalizedService := service

		// If service doesn't have a restart policy but should inherit from global config
		if service.Restart == nil && service.Cron == "" {
			normalizedService.Restart = config.Restart
		}

		normalizedConfig.Services[name] = normalizedService
	}

	return normalizedConfig
}
