package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/adhocore/gronx"
)

func DeserializeConfig(data any, envMapping func(string) string, gron *gronx.Gronx) (*Config, error) {
	record, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected map[string]any, got %T", ErrInvalidConfig, data)
	}
	cfg := &Config{
		DefaultInitialDelay: 5 * time.Second,
		DefaultGracePeriod:  30 * time.Second,
		Services:            make(map[string]*ServiceEntry),
	}

	if delayValue, ok := record["delay"]; ok {
		delay, err := parseDuration(delayValue, envMapping, false)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid delay value %q: %v", ErrInvalidConfig, delayValue, err)
		}
		cfg.DefaultInitialDelay = delay
	}

	if graceValue, ok := record["grace"]; ok {
		grace, err := parseDuration(graceValue, envMapping, false)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid grace value %q: %v", ErrInvalidConfig, graceValue, err)
		}
		cfg.DefaultGracePeriod = grace
	}

	if servicesValue, ok := record["services"]; ok {
		services, ok := servicesValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: expected map[string]any for services, got %T", ErrInvalidConfig, servicesValue)
		}
		if len(services) == 0 {
			return nil, fmt.Errorf("%w: services section cannot be empty", ErrInvalidConfig)
		}
		for name, entryData := range services {
			if !PatternServiceName.MatchString(name) {
				return nil, fmt.Errorf("%w: invalid service name %q", ErrInvalidConfig, name)
			}
			entry, err := deserializeServiceEntry(cfg, entryData, envMapping, gron)
			if err != nil {
				return nil, fmt.Errorf("%w: error deserialize service %s: %w", ErrInvalidConfig, name, err)
			}
			cfg.Services[name] = entry
		}
	} else {
		return nil, fmt.Errorf("%w: missing 'services' section", ErrInvalidConfig)
	}

	if adminValue, ok := record["admin"]; ok {
		adminEntry, err := deserializeAdminEntry(adminValue, envMapping)
		if err != nil {
			return nil, fmt.Errorf("%w: error deserialize admin entry: %v", ErrInvalidConfig, err)
		}
		cfg.Admin = adminEntry
	} else {
		cfg.Admin = nil
	}

	return cfg, nil
}

func deserializeServiceEntry(config *Config, data any, envMapping func(string) string, gron *gronx.Gronx) (*ServiceEntry, error) {
	record, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map[string]any, got %T", data)
	}

	restartValue, hasRestart := record["restart"]
	cronValue, hasCron := record["cron"]

	if hasRestart && hasCron {
		return nil, fmt.Errorf("service entry cannot have both 'restart' and 'cron' fields")
	}

	if !hasRestart && !hasCron {
		restartValue = true
		hasRestart = true
	}

	service := &ServiceEntry{
		Env: make(map[string]string),
	}

	if commandValue, ok := record["cmd"]; ok {
		cmd, err := parseCommand(commandValue, envMapping)
		if err != nil {
			return nil, fmt.Errorf("invalid command value %q: %w", commandValue, err)
		}
		service.Command = cmd
	} else {
		return nil, fmt.Errorf("missing 'cmd' field in service entry")
	}

	if workingDirValue, ok := record["workDir"]; ok {
		if workingDir, ok := workingDirValue.(string); ok {
			workingDir = os.Expand(workingDir, envMapping)
			service.WorkingDir = workingDir
		} else {
			return nil, fmt.Errorf("expected string for 'workDir', got %T", workingDirValue)
		}
	}

	if graceValue, ok := record["grace"]; ok {
		grace, err := parseDuration(graceValue, envMapping, false)
		if err != nil {
			return nil, fmt.Errorf("invalid grace value %q: %w", graceValue, err)
		}
		service.GracePeriod = grace
	} else {
		service.GracePeriod = config.DefaultGracePeriod
	}

	if envValue, ok := record["env"]; ok {
		envMap, ok := envValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected map[string]any for 'env', got %T", envValue)
		}
		for key, value := range envMap {
			if !PatternEnvName.MatchString(key) {
				return nil, fmt.Errorf("invalid environment variable name %q", key)
			}
			if strValue, ok := value.(string); ok {
				strValue = os.Expand(strValue, envMapping)
				service.Env[key] = strValue
			} else {
				return nil, fmt.Errorf("expected string for environment variable %q, got %T", key, value)
			}
		}
	}

	if optionalValue, ok := record["optional"]; ok {
		if optional, ok := optionalValue.(bool); ok {
			service.Optional = optional
		} else {
			return nil, fmt.Errorf("expected bool for 'optional', got %T", optionalValue)
		}
	} else {
		service.Optional = false
	}

	if hasRestart {
		service.Type = ServiceTypeService
		restartPolicy, err := deserializeRestartPolicy(config, restartValue, envMapping)
		if err != nil {
			return nil, fmt.Errorf("invalid restart policy value %q: %w", restartValue, err)
		}
		service.Restart = restartPolicy
	} else {
		service.Type = ServiceTypeCron
		if cron, ok := cronValue.(string); ok {
			cron = os.Expand(cron, envMapping)
			if !gron.IsValid(cron) {
				return nil, fmt.Errorf("invalid cron expression %q", cron)
			}
			service.Cron = cron
		} else {
			return nil, fmt.Errorf("expected string for 'cron', got %T", cronValue)
		}
		if singleValue, ok := record["single"]; ok {
			if single, ok := singleValue.(bool); ok {
				service.Single = single
			} else {
				return nil, fmt.Errorf("expected bool for 'single', got %T", singleValue)
			}
		} else {
			service.Single = false
		}
	}

	return service, nil
}

func deserializeRestartPolicy(config *Config, data any, envMapping func(string) string) (*RestartPolicy, error) {
	switch data := data.(type) {
	case string:
		data = os.Expand(data, envMapping)
		switch data {
		case RestartPolicyAlways:
			return &RestartPolicy{
				Mode:         RestartPolicyAlways,
				Exponential:  false,
				InitialDelay: config.DefaultInitialDelay,
			}, nil
		case RestartPolicyOnFailure:
			return &RestartPolicy{
				Mode:         RestartPolicyOnFailure,
				Exponential:  false,
				InitialDelay: config.DefaultInitialDelay,
			}, nil
		case RestartPolicyNever:
			return &RestartPolicy{
				Mode:         RestartPolicyNever,
				Exponential:  false,
				InitialDelay: config.DefaultInitialDelay,
			}, nil
		case "exponential":
			return &RestartPolicy{
				Mode:         RestartPolicyAlways,
				Exponential:  true,
				InitialDelay: config.DefaultInitialDelay,
			}, nil
		}
	case bool:
		if data {
			return &RestartPolicy{
				Mode:         RestartPolicyAlways,
				Exponential:  false,
				InitialDelay: config.DefaultInitialDelay,
			}, nil
		} else {
			return &RestartPolicy{
				Mode:         RestartPolicyNever,
				Exponential:  false,
				InitialDelay: config.DefaultInitialDelay,
			}, nil
		}
	case map[string]any:
		policy := &RestartPolicy{}
		if mode, ok := data["mode"]; ok {
			switch modeStr := mode.(type) {
			case string:
				modeStr = os.Expand(modeStr, envMapping)
				if !PatternRestartPolicy.MatchString(modeStr) {
					return nil, fmt.Errorf("invalid restart mode %q", modeStr)
				}
				policy.Mode = modeStr
			default:
				return nil, fmt.Errorf("expected string for 'mode', got %T", mode)
			}
		} else {
			return nil, fmt.Errorf("missing 'mode' field in restart policy")
		}

		if exponential, ok := data["exponential"]; ok {
			if exp, ok := exponential.(bool); ok {
				policy.Exponential = exp
			} else {
				return nil, fmt.Errorf("expected bool for 'exponential', got %T", exponential)
			}
		} else {
			policy.Exponential = false // Default to false if not specified
		}
		if delayValue, ok := data["delay"]; ok {
			delay, err := parseDuration(delayValue, envMapping, false)
			if err != nil {
				return nil, fmt.Errorf("invalid delay value %q: %w", delayValue, err)
			}
			policy.InitialDelay = delay
		} else {
			policy.InitialDelay = config.DefaultInitialDelay // Default to config value if not specified
		}

		if maxDelayValue, ok := data["maxDelay"]; ok {
			maxDelay, err := parseDuration(maxDelayValue, envMapping, true)
			if err != nil {
				return nil, fmt.Errorf("invalid maxDelay value %q: %w", maxDelayValue, err)
			}
			policy.MaxDelay = maxDelay
		} else {
			policy.MaxDelay = 0
		}

		if maxRetriesValue, ok := data["maxRetries"]; ok {
			switch maxRetries := maxRetriesValue.(type) {
			case string:
				if maxRetries != "infinity" {
					return nil, fmt.Errorf("expected 'infinity' or number for 'maxRetries', got %q", maxRetries)
				}
				policy.MaxRetries = -1
			case int64:
				if maxRetries <= 0 {
					return nil, fmt.Errorf("negative or zero value for 'maxRetries' is not allowed: %d", maxRetries)
				}
				policy.MaxRetries = int(maxRetries)
			default:
				return nil, fmt.Errorf("expected string or int for 'maxRetries', got %T", maxRetriesValue)
			}
		} else {
			policy.MaxRetries = 0
		}

		return policy, nil
	}
	return nil, fmt.Errorf("expected string, bool or map[string]any for restart policy, got %T", data)
}

func deserializeAdminEntry(data any, envMapping func(string) string) (*AdminEntry, error) {
	entry := &AdminEntry{
		Host: "localhost",
		Port: 8080,
		Unix: "",
	}
	switch data := data.(type) {
	case bool:
		if data {
			return entry, nil
		}
		return nil, nil
	case map[string]any:
		if enabled, ok := data["enabled"]; ok {
			switch val := enabled.(type) {
			case bool:
				if val {
					return entry, nil
				}
				return nil, nil
			case string:
				val = os.Expand(val, envMapping)
				if val == "true" || val == "1" {
					return entry, nil
				}
				return nil, nil
			}
		}
		if unix, ok := data["unix"]; ok {
			if val, ok := unix.(string); ok {
				val = os.Expand(val, envMapping)
				entry.Unix = val
			} else {
				return nil, fmt.Errorf("expected string for 'unix', got %T", unix)
			}
		}
		if host, ok := data["host"]; ok {
			if val, ok := host.(string); ok {
				val = os.Expand(val, envMapping)
				entry.Host = val
			} else {
				return nil, fmt.Errorf("expected string for 'host', got %T", host)
			}
		}
		if port, ok := data["port"]; ok {
			switch val := port.(type) {
			case int64:
				if val <= 0 || val > 65535 {
					return nil, fmt.Errorf("invalid port number %d, must be between 1 and 65535", val)
				}
				entry.Port = int(val)
			case string:
				val = os.Expand(val, envMapping)
				if strings.TrimSpace(val) == "" {
					return nil, fmt.Errorf("port cannot be empty")
				}
				if portNum, err := strconv.Atoi(val); err == nil && portNum > 0 && portNum <= 65535 {
					entry.Port = portNum
				} else {
					return nil, fmt.Errorf("invalid port string %q: %v", val, err)
				}
			default:
				return nil, fmt.Errorf("expected number or string for 'port', got %T", port)
			}
		}
		return entry, nil
	}
	return nil, fmt.Errorf("expected bool or map[string]any for admin entry, got %T", data)
}

func parseDuration(value any, envMapping func(string) string, allowInfinity bool) (time.Duration, error) {
	switch v := value.(type) {
	case string:
		v = os.Expand(v, envMapping)
		if allowInfinity && v == "infinity" {
			return time.Duration(-1), nil
		}
		if d, err := time.ParseDuration(v); err == nil {
			return d, nil
		} else {
			return 0, err
		}
	case float64:
		if v <= 0 {
			return 0, fmt.Errorf("negative duration value %f is not allowed", v)
		}
		return time.Duration(v * float64(time.Second)), nil
	case int64:
		if v <= 0 {
			return 0, fmt.Errorf("negative duration value %d is not allowed", v)
		}
		return time.Duration(v) * time.Second, nil
	}
	return 0, fmt.Errorf("expected string or number for duration, got %T", value)
}

func parseCommand(command any, envMapping func(string) string) ([]string, error) {
	switch cmd := command.(type) {
	case string:
		cmd = os.Expand(cmd, envMapping)
		return strings.Fields(cmd), nil
	case []any:
		var result []string
		for _, part := range cmd {
			if str, ok := part.(string); ok {
				str = os.Expand(str, envMapping)
				result = append(result, str)
			} else {
				return nil, fmt.Errorf("expected string in command array, got %T", part)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected string or []string for command, got %T", command)
	}
}
