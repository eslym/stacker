package supervisor

import (
	"time"

	"github.com/eslym/stacker/pkg/config"
)

const (
	ServiceTypeProcess = iota
	ServiceTypeCron
)

const (
	RestartModeAlways = iota
	RestartModeOnFailure
	RestartModeNever
)

type ServiceType int

type RestartMode int

type Service interface {
	GetName() string
	GetType() ServiceType
	AsProcess() ProcessService
	AsCron() CronService

	OnStdout(listener chan string)
	OnStderr(listener chan string)
	OffStdout(listener chan string)
	OffStderr(listener chan string)

	GetSerializedStats() map[string]any
}

type RestartPolicy interface {
	GetMode() RestartMode
	IsExponential() bool
	GetMaxRetries() int
	GetInitialDelay() time.Duration
	GetMaxDelay() time.Duration
}

type ProcessService interface {
	Service
	IsRunning() bool
	GetRetries() int
	GetRestartPolicy() RestartPolicy
	GetLastExitCode() int

	Start() error
	Stop(user bool) error
	Kill() error
	Restart() error
}

type CronService interface {
	Service
	IsSingle() bool
	IsEnabled() bool
	GetSchedule() string
	SetEnabled(enabled bool)
	StopAll() error
	GetCommand() []string
	GetWorkDir() string
	GetEnv() map[string]string
	GetProcesses() []Process
	Run(t time.Time)
}

func (t ServiceType) String() string {
	switch t {
	case ServiceTypeProcess:
		return "process"
	case ServiceTypeCron:
		return "cron"
	default:
		return "unknown"
	}
}

func ParseRestartMode(s string) RestartMode {
	switch s {
	case "always":
		return RestartModeAlways
	case "on-failure":
		return RestartModeOnFailure
	case "never":
		return RestartModeNever
	default:
		return -1 // Unknown mode
	}
}

func (r RestartMode) String() string {
	switch r {
	case RestartModeAlways:
		return "always"
	case RestartModeOnFailure:
		return "on-failure"
	case RestartModeNever:
		return "never"
	default:
		return "unknown"
	}
}

// restartPolicy implements the RestartPolicy interface
type restartPolicy struct {
	mode         RestartMode
	exponential  bool
	maxRetries   int
	initialDelay time.Duration
	maxDelay     time.Duration
}

// GetMode returns the restart mode
func (r *restartPolicy) GetMode() RestartMode {
	return r.mode
}

// IsExponential returns whether exponential backoff is enabled
func (r *restartPolicy) IsExponential() bool {
	return r.exponential
}

// GetMaxRetries returns the maximum number of retries
// -1 means infinite retries
func (r *restartPolicy) GetMaxRetries() int {
	return r.maxRetries
}

// GetInitialDelay returns the initial delay before restarting
func (r *restartPolicy) GetInitialDelay() time.Duration {
	return r.initialDelay
}

// GetMaxDelay returns the maximum delay before restarting
// 0 means no maximum
func (r *restartPolicy) GetMaxDelay() time.Duration {
	return r.maxDelay
}

// NewRestartPolicy creates a new RestartPolicy from a config.RestartPolicy
func NewRestartPolicy(config *config.RestartPolicy) RestartPolicy {
	if config == nil {
		return nil
	}

	return &restartPolicy{
		mode:         ParseRestartMode(config.Mode),
		exponential:  config.Exponential,
		maxRetries:   config.MaxRetries,
		initialDelay: config.InitialDelay,
		maxDelay:     config.MaxDelay,
	}
}
