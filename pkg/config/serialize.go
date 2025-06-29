package config

func (rp *RestartPolicy) Serialize() map[string]any {
	record := map[string]any{
		"mode":        rp.Mode,
		"exponential": rp.Exponential,
		"delay":       rp.InitialDelay.String(),
	}
	if rp.MaxDelay > 0 {
		record["maxDelay"] = rp.MaxDelay.String()
	} else {
		record["maxDelay"] = "infinity"
	}
	if rp.MaxRetries > 0 {
		record["maxRetries"] = rp.MaxRetries
	} else {
		record["maxRetries"] = "infinity"
	}
	return record
}

func (se *ServiceEntry) Serialize() map[string]any {
	record := map[string]any{
		"cmd": se.Command,
	}
	if se.WorkingDir != "" {
		record["workDir"] = se.WorkingDir
	}
	record["optional"] = se.Optional
	if len(se.Env) > 0 {
		record["env"] = make(map[string]string)
		for k, v := range se.Env {
			record["env"].(map[string]string)[k] = v
		}
	}
	if se.Restart != nil {
		record["restart"] = se.Restart.Serialize()
	} else {
		record["cron"] = se.Cron
		record["single"] = se.Single
	}
	record["grace"] = se.GracePeriod.String()
	return record
}

func (ae *AdminEntry) Serialize() map[string]any {
	record := make(map[string]any)
	if ae.Unix != "" {
		record["unix"] = ae.Unix
	} else {
		record["host"] = ae.Host
		record["port"] = ae.Port
	}
	return record
}

func (c *Config) Serialize() map[string]any {
	record := map[string]any{
		"delay": c.DefaultInitialDelay.String(),
		"grace": c.DefaultGracePeriod.String(),
	}
	if c.Admin != nil {
		record["admin"] = c.Admin.Serialize()
	} else {
		record["admin"] = false
	}
	record["services"] = make(map[string]any)
	for name, entry := range c.Services {
		record["services"].(map[string]any)[name] = entry.Serialize()
	}
	return record
}
