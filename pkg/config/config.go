package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// FindConfigFile tries to find a config file in multiple locations
func FindConfigFile(configPath string) (string, error) {
	// If a specific path is provided, use it
	if configPath != "" {
		return configPath, nil
	}

	// Try to find config file in the following order:

	// 1. Current directory: ./stacker.{json,yml,yaml}
	currentDirPaths := []string{
		"stacker.json",
		"stacker.yml",
		"stacker.yaml",
	}

	for _, path := range currentDirPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// 2. Environment variable: STACKER_CONFIG_PATH
	if envPath := os.Getenv("STACKER_CONFIG_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath, nil
		}
	}

	// 3. User config directory: $HOME/.config/stacker/config.{json,yml,yaml}
	homeDir, err := os.UserHomeDir()
	if err == nil {
		homePaths := []string{
			filepath.Join(homeDir, ".config", "stacker", "config.json"),
			filepath.Join(homeDir, ".config", "stacker", "config.yml"),
			filepath.Join(homeDir, ".config", "stacker", "config.yaml"),
		}

		for _, path := range homePaths {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	// 4. OS-specific fallback path
	defaultBasePath := GetDefaultConfigPath()
	defaultPaths := []string{
		defaultBasePath + ".json",
		defaultBasePath + ".yml",
		defaultBasePath + ".yaml",
	}

	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// No config file found
	return "", fmt.Errorf("no config file found in any of the default locations")
}

// LoadConfig loads the configuration from a file
func LoadConfig(configPath string) (*Config, error) {
	// Find the config file
	foundPath, err := FindConfigFile(configPath)
	if err != nil {
		return nil, err
	}

	// Read the file
	data, err := os.ReadFile(foundPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %s", err)
	}

	var config Config

	// Determine file type by extension
	ext := strings.ToLower(filepath.Ext(foundPath))
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

	// Apply environment variable substitution
	applyEnvSubstitution(&config)

	// Validate the config
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// applyEnvSubstitution applies environment variable substitution to the config
func applyEnvSubstitution(config *Config) {
	// Apply to global config fields
	config.Restart = substituteEnvVars(config.Restart)
	config.Grace = substituteEnvVars(config.Grace)

	// Apply to services
	for name, service := range config.Services {
		// Handle Cmd field which can be string or []interface{}
		switch cmd := service.Cmd.(type) {
		case string:
			service.Cmd = substituteEnvVars(cmd)
		case []interface{}:
			service.Cmd = substituteEnvVarsInInterface(cmd)
		}

		// Handle other string fields
		service.Cwd = substituteEnvVars(service.Cwd)
		service.Grace = substituteEnvVars(service.Grace)
		service.Cron = substituteEnvVars(service.Cron)

		// Handle conflict field which can be string or []interface{}
		if service.Conflict != nil {
			service.Conflict = substituteEnvVarsInInterface(service.Conflict)
		}

		// Handle environment variables in the service's environment
		if service.Env != nil {
			for key, val := range service.Env {
				service.Env[key] = substituteEnvVars(val)
			}
		}

		// Update the service in the config
		config.Services[name] = service
	}
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

// substituteEnvVars replaces environment variables in a string
// Supports $VAR, ${VAR}, and ${VAR:-default} syntax
func substituteEnvVars(input string) string {
	if input == "" {
		return input
	}

	// Pattern for ${VAR:-default}
	defaultPattern := regexp.MustCompile(`\${([^{}:]+):-([^{}]*)}`)
	// Pattern for ${VAR}
	bracesPattern := regexp.MustCompile(`\${([^{}]+)}`)
	// Pattern for $VAR
	simplePattern := regexp.MustCompile(`\$([a-zA-Z0-9_]+)`)

	// Replace ${VAR:-default} with value or default
	result := defaultPattern.ReplaceAllStringFunc(input, func(match string) string {
		// Extract variable name and default value
		parts := defaultPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		varName := parts[1]
		defaultValue := parts[2]

		// Get environment variable value or use default
		value := os.Getenv(varName)
		if value == "" {
			return defaultValue
		}
		return value
	})

	// Replace ${VAR} with value
	result = bracesPattern.ReplaceAllStringFunc(result, func(match string) string {
		// Extract variable name
		parts := bracesPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}

		varName := parts[1]

		// Get environment variable value
		return os.Getenv(varName)
	})

	// Replace $VAR with value
	result = simplePattern.ReplaceAllStringFunc(result, func(match string) string {
		// Extract variable name
		parts := simplePattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}

		varName := parts[1]

		// Get environment variable value
		return os.Getenv(varName)
	})

	return result
}

// substituteEnvVarsInInterface recursively substitutes environment variables in interface{} values
func substituteEnvVarsInInterface(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return substituteEnvVars(v)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = substituteEnvVarsInInterface(item)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, val := range v {
			result[key] = substituteEnvVarsInInterface(val)
		}
		return result
	default:
		return v
	}
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
