package config

import (
	"errors"
	"regexp"
	"time"
)

// ...existing code: constants, regex patterns...

const (
	ServiceTypeService = "service"
	ServiceTypeCron    = "cron"
)

const (
	RestartPolicyAlways    = "always"
	RestartPolicyOnFailure = "on-failure"
	RestartPolicyNever     = "never"
)

var PatternServiceName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var PatternEnvName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var PatternRestartPolicy = regexp.MustCompile(`^(always|on-failure|never|exponential)$`)

var ErrInvalidConfig = errors.New("invalid config")

type RestartPolicy struct {
	Mode         string
	Exponential  bool
	InitialDelay time.Duration
	MaxDelay     time.Duration
	MaxRetries   int
}

type ServiceEntry struct {
	Type        string
	Command     []string
	WorkingDir  string
	Optional    bool
	Env         map[string]string
	Restart     *RestartPolicy
	Cron        string
	Single      bool
	GracePeriod time.Duration
}

type AdminEntry struct {
	Host string
	Port int
	Unix string
}

type Config struct {
	DefaultInitialDelay time.Duration
	DefaultGracePeriod  time.Duration
	Admin               *AdminEntry
	Services            map[string]*ServiceEntry
}
