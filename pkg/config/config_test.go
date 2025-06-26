package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eslym/stacker/pkg/config"
)

func TestLoadConfig_YAML(t *testing.T) {
	// Create a temporary YAML config file
	content := `
restart: 10s
grace: 15s
services:
  web:
    cmd: ["php", "artisan", "serve"]
    cwd: /var/www/html
    restart: true
  cron:
    cmd: ["php", "artisan", "schedule:run"]
    cron: "* * * * *"
  queue:
    cmd: ["php", "artisan", "queue:work"]
    restart: true
    conflict: horizon
    optional: true
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	// Load the config
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the config
	if cfg.Restart != "10s" {
		t.Errorf("Expected restart to be 10s, got %s", cfg.Restart)
	}
	if cfg.Grace != "15s" {
		t.Errorf("Expected grace to be 15s, got %s", cfg.Grace)
	}
	if len(cfg.Services) != 3 {
		t.Errorf("Expected 3 services, got %d", len(cfg.Services))
	}

	// Verify web service
	web, ok := cfg.Services["web"]
	if !ok {
		t.Errorf("Expected web service to exist")
	} else {
		cmdArray, ok := web.Cmd.([]interface{})
		if !ok {
			t.Errorf("Expected web.Cmd to be an array")
		} else if len(cmdArray) != 3 {
			t.Errorf("Expected web.Cmd to have 3 elements, got %d", len(cmdArray))
		}
		if web.Cwd != "/var/www/html" {
			t.Errorf("Expected web.Cwd to be /var/www/html, got %s", web.Cwd)
		}
		if web.Restart != true {
			t.Errorf("Expected web.Restart to be true, got %v", web.Restart)
		}
	}

	// Verify cron service
	cron, ok := cfg.Services["cron"]
	if !ok {
		t.Errorf("Expected cron service to exist")
	} else {
		if cron.Cron != "* * * * *" {
			t.Errorf("Expected cron.Cron to be * * * * *, got %s", cron.Cron)
		}
	}

	// Verify queue service
	queue, ok := cfg.Services["queue"]
	if !ok {
		t.Errorf("Expected queue service to exist")
	} else {
		if queue.Optional != true {
			t.Errorf("Expected queue.Optional to be true, got %v", queue.Optional)
		}
		if queue.Conflict != "horizon" {
			t.Errorf("Expected queue.Conflict to be horizon, got %v", queue.Conflict)
		}
	}
}

func TestLoadConfig_JSON(t *testing.T) {
	// Create a temporary JSON config file
	content := `{
  "restart": "10s",
  "grace": "15s",
  "services": {
    "web": {
      "cmd": ["php", "artisan", "serve"],
      "cwd": "/var/www/html",
      "restart": true
    },
    "cron": {
      "cmd": ["php", "artisan", "schedule:run"],
      "cron": "* * * * *"
    },
    "queue": {
      "cmd": ["php", "artisan", "queue:work"],
      "restart": true,
      "conflict": "horizon",
      "optional": true
    }
  }
}`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	// Load the config
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the config
	if cfg.Restart != "10s" {
		t.Errorf("Expected restart to be 10s, got %s", cfg.Restart)
	}
	if cfg.Grace != "15s" {
		t.Errorf("Expected grace to be 15s, got %s", cfg.Grace)
	}
	if len(cfg.Services) != 3 {
		t.Errorf("Expected 3 services, got %d", len(cfg.Services))
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := config.LoadConfig("nonexistent.yaml")
	if err == nil {
		t.Errorf("Expected error for nonexistent file, got nil")
	}
}

func TestLoadConfig_InvalidFormat(t *testing.T) {
	// Create a temporary file with invalid extension
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.txt")
	if err := os.WriteFile(configPath, []byte("invalid"), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	_, err := config.LoadConfig(configPath)
	if err == nil {
		t.Errorf("Expected error for invalid format, got nil")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	// Create a temporary YAML file with invalid content
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	_, err := config.LoadConfig(configPath)
	if err == nil {
		t.Errorf("Expected error for invalid YAML, got nil")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	// Create a temporary JSON file with invalid content
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{invalid: json}"), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	_, err := config.LoadConfig(configPath)
	if err == nil {
		t.Errorf("Expected error for invalid JSON, got nil")
	}
}

func TestValidateConfig_NoServices(t *testing.T) {
	// Create a temporary YAML file with no services
	content := `
restart: 10s
grace: 15s
services: {}
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	_, err := config.LoadConfig(configPath)
	if err == nil {
		t.Errorf("Expected error for no services, got nil")
	}
}

func TestValidateConfig_EmptyCommand(t *testing.T) {
	// Create a temporary YAML file with empty command
	content := `
restart: 10s
grace: 15s
services:
  web:
    cmd: ""
    cwd: /var/www/html
    restart: true
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	_, err := config.LoadConfig(configPath)
	if err == nil {
		t.Errorf("Expected error for empty command, got nil")
	}
}

func TestValidateConfig_EmptyCommandArray(t *testing.T) {
	// Create a temporary YAML file with empty command array
	content := `
restart: 10s
grace: 15s
services:
  web:
    cmd: []
    cwd: /var/www/html
    restart: true
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	_, err := config.LoadConfig(configPath)
	if err == nil {
		t.Errorf("Expected error for empty command array, got nil")
	}
}

func TestValidateConfig_InvalidCommandType(t *testing.T) {
	// Create a temporary YAML file with invalid command type
	content := `
restart: 10s
grace: 15s
services:
  web:
    cmd: 123
    cwd: /var/www/html
    restart: true
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	_, err := config.LoadConfig(configPath)
	if err == nil {
		t.Errorf("Expected error for invalid command type, got nil")
	}
}

func TestValidateConfig_RestartAndCron(t *testing.T) {
	// Create a temporary YAML file with both restart and cron
	content := `
restart: 10s
grace: 15s
services:
  web:
    cmd: ["php", "artisan", "serve"]
    cwd: /var/www/html
    restart: true
    cron: "* * * * *"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	_, err := config.LoadConfig(configPath)
	if err == nil {
		t.Errorf("Expected error for both restart and cron, got nil")
	}
}

func TestDefaultValues(t *testing.T) {
	// Create a temporary YAML file with minimal configuration
	content := `
services:
  web:
    cmd: ["php", "artisan", "serve"]
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	// Load the config
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify default values
	if cfg.Restart != "5s" {
		t.Errorf("Expected default restart to be 5s, got %s", cfg.Restart)
	}
	if cfg.Grace != "5s" {
		t.Errorf("Expected default grace to be 5s, got %s", cfg.Grace)
	}
}

func TestEnvVarSubstitution(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("SLEEP_FOR", "60")
	os.Setenv("APP_PORT", "8080")
	os.Setenv("APP_HOST", "localhost")
	os.Setenv("LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("SLEEP_FOR")
		os.Unsetenv("APP_PORT")
		os.Unsetenv("APP_HOST")
		os.Unsetenv("LOG_LEVEL")
	}()

	// Create a temporary YAML file with environment variables
	content := `
restart: $RESTART_TIME
grace: ${GRACE_TIME:-10s}
services:
  sleep:
    cmd: ["sleep", "$SLEEP_FOR"]
    env:
      PORT: "${APP_PORT}"
      HOST: "$APP_HOST"
      LOG_LEVEL: "${LOG_LEVEL:-info}"
      DEBUG: "${DEBUG:-false}"
  web:
    cmd: "php -S ${APP_HOST}:${APP_PORT}"
    cwd: /var/www/html
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temporary config file: %v", err)
	}

	// Load the config
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify environment variable substitution in global config
	if cfg.Restart != "5s" { // RESTART_TIME is not set, should be default "5s" from validateConfig
		t.Errorf("Expected restart to be '5s' (default), got %s", cfg.Restart)
	}
	if cfg.Grace != "10s" { // GRACE_TIME is not set, should use default from env var substitution
		t.Errorf("Expected grace to be 10s (default), got %s", cfg.Grace)
	}

	// Verify environment variable substitution in sleep service
	sleep, ok := cfg.Services["sleep"]
	if !ok {
		t.Errorf("Expected sleep service to exist")
	} else {
		cmdArray, ok := sleep.Cmd.([]interface{})
		if !ok {
			t.Errorf("Expected sleep.Cmd to be an array")
		} else if len(cmdArray) != 2 {
			t.Errorf("Expected sleep.Cmd to have 2 elements, got %d", len(cmdArray))
		} else if cmdArray[1] != "60" {
			t.Errorf("Expected sleep.Cmd[1] to be '60', got %v", cmdArray[1])
		}

		// Verify environment variables in service.Env
		if sleep.Env["PORT"] != "8080" {
			t.Errorf("Expected sleep.Env[PORT] to be '8080', got %s", sleep.Env["PORT"])
		}
		if sleep.Env["HOST"] != "localhost" {
			t.Errorf("Expected sleep.Env[HOST] to be 'localhost', got %s", sleep.Env["HOST"])
		}
		if sleep.Env["LOG_LEVEL"] != "debug" {
			t.Errorf("Expected sleep.Env[LOG_LEVEL] to be 'debug', got %s", sleep.Env["LOG_LEVEL"])
		}
		if sleep.Env["DEBUG"] != "false" {
			t.Errorf("Expected sleep.Env[DEBUG] to be 'false' (default), got %s", sleep.Env["DEBUG"])
		}
	}

	// Verify environment variable substitution in web service
	web, ok := cfg.Services["web"]
	if !ok {
		t.Errorf("Expected web service to exist")
	} else {
		cmdStr, ok := web.Cmd.(string)
		if !ok {
			t.Errorf("Expected web.Cmd to be a string")
		} else if cmdStr != "php -S localhost:8080" {
			t.Errorf("Expected web.Cmd to be 'php -S localhost:8080', got %s", cmdStr)
		}
	}
}
