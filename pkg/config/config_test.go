package config

import (
	"reflect"
	"testing"
	"time"

	"github.com/adhocore/gronx"
)

func dummyEnv(key string) string {
	switch key {
	case "FOO":
		return "bar"
	case "PORT":
		return "1234"
	default:
		return ""
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		value         any
		allowInfinity bool
		want          time.Duration
		wantErr       bool
	}{
		{"2s", false, 2 * time.Second, false},
		{"infinity", true, -1, false},
		{float64(3), false, 3 * time.Second, false},
		{int64(4), false, 4 * time.Second, false},
		{"bad", false, 0, true},
		{-1, false, 0, true},
	}

	for _, tt := range tests {
		got, err := parseDuration(tt.value, dummyEnv, tt.allowInfinity)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseDuration(%v) error = %v, wantErr %v", tt.value, err, tt.wantErr)
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("parseDuration(%v) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestParseCommand(t *testing.T) {
	cmd, err := parseCommand("echo $FOO", dummyEnv)
	if err != nil || !reflect.DeepEqual(cmd, []string{"echo", "bar"}) {
		t.Errorf("parseCommand string failed: %v, %v", cmd, err)
	}
	cmd, err = parseCommand([]any{"ls", "$FOO"}, dummyEnv)
	if err != nil || !reflect.DeepEqual(cmd, []string{"ls", "bar"}) {
		t.Errorf("parseCommand array failed: %v, %v", cmd, err)
	}
	_, err = parseCommand(123, dummyEnv)
	if err == nil {
		t.Error("parseCommand should fail for non-string/array")
	}
}

func TestDeserializeRestartPolicy(t *testing.T) {
	cfg := &Config{DefaultInitialDelay: 1 * time.Second}
	// string
	rp, err := deserializeRestartPolicy(cfg, "always", dummyEnv)
	if err != nil || rp.Mode != RestartPolicyAlways {
		t.Errorf("deserializeRestartPolicy string failed: %v, %v", rp, err)
	}
	// bool
	rp, err = deserializeRestartPolicy(cfg, true, dummyEnv)
	if err != nil || rp.Mode != RestartPolicyAlways {
		t.Errorf("deserializeRestartPolicy bool failed: %v, %v", rp, err)
	}
	// map
	rp, err = deserializeRestartPolicy(cfg, map[string]any{
		"mode":        "on-failure",
		"exponential": true,
		"delay":       "2s",
		"maxDelay":    "3s",
		"maxRetries":  int64(5),
	}, dummyEnv)
	if err != nil || rp.Mode != RestartPolicyOnFailure || !rp.Exponential || rp.InitialDelay != 2*time.Second || rp.MaxDelay != 3*time.Second || rp.MaxRetries != 5 {
		t.Errorf("deserializeRestartPolicy map failed: %v, %v", rp, err)
	}
	// error cases
	_, err = deserializeRestartPolicy(cfg, map[string]any{"mode": 123}, dummyEnv)
	if err == nil {
		t.Error("deserializeRestartPolicy should fail for non-string mode")
	}
	_, err = deserializeRestartPolicy(cfg, 123, dummyEnv)
	if err == nil {
		t.Error("deserializeRestartPolicy should fail for invalid type")
	}
}

func TestDeserializeAdminEntry(t *testing.T) {
	ae, err := deserializeAdminEntry(true, dummyEnv)
	if err != nil || ae == nil {
		t.Errorf("deserializeAdminEntry bool failed: %v, %v", ae, err)
	}
	ae, err = deserializeAdminEntry(map[string]any{
		"host": "127.0.0.1",
		"port": int64(9000),
	}, dummyEnv)
	if err != nil || ae.Host != "127.0.0.1" || ae.Port != 9000 {
		t.Errorf("deserializeAdminEntry map failed: %v, %v", ae, err)
	}
	ae, err = deserializeAdminEntry(map[string]any{
		"unix": "/tmp/socket",
	}, dummyEnv)
	if err != nil || ae.Unix != "/tmp/socket" {
		t.Errorf("deserializeAdminEntry unix failed: %v, %v", ae, err)
	}
	_, err = deserializeAdminEntry(123, dummyEnv)
	if err == nil {
		t.Error("deserializeAdminEntry should fail for invalid type")
	}
}

func TestDeserializeConfig(t *testing.T) {
	gron := gronx.New()
	data := map[string]any{
		"delay": "1s",
		"grace": "2s",
		"services": map[string]any{
			"svc1": map[string]any{
				"cmd":      "echo hi",
				"optional": true,
				"env": map[string]any{
					"FOO": "bar",
				},
				"restart": true,
			},
			"cronjob": map[string]any{
				"cmd":  "ls",
				"cron": "* * * * *",
			},
		},
		"admin": map[string]any{
			"host": "localhost",
			"port": int64(8080),
		},
	}
	cfg, err := DeserializeConfig(data, dummyEnv, gron)
	if err != nil {
		t.Fatalf("DeserializeConfig failed: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(cfg.Services))
	}
	if cfg.DefaultInitialDelay != time.Second || cfg.DefaultGracePeriod != 2*time.Second {
		t.Errorf("unexpected delay/grace: %v, %v", cfg.DefaultInitialDelay, cfg.DefaultGracePeriod)
	}
	// error: missing services
	_, err = DeserializeConfig(map[string]any{}, dummyEnv, gron)
	if err == nil {
		t.Error("DeserializeConfig should fail for missing services")
	}
	// error: invalid service name
	_, err = DeserializeConfig(map[string]any{
		"services": map[string]any{
			"bad name!": map[string]any{"cmd": "ls", "restart": true},
		},
	}, dummyEnv, gron)
	if err == nil {
		t.Error("DeserializeConfig should fail for invalid service name")
	}
}

func TestServiceEntrySerialize(t *testing.T) {
	se := &ServiceEntry{
		Command:    []string{"ls"},
		WorkingDir: "/tmp",
		Optional:   true,
		Env:        map[string]string{"FOO": "bar"},
		Restart: &RestartPolicy{
			Mode:         RestartPolicyAlways,
			Exponential:  false,
			InitialDelay: 1 * time.Second,
			MaxDelay:     2 * time.Second,
			MaxRetries:   3,
		},
		GracePeriod: 5 * time.Second,
	}
	m := se.Serialize()
	if m["workDir"] != "/tmp" || m["optional"] != true {
		t.Errorf("ServiceEntry.Serialize failed: %v", m)
	}
}

func TestRestartPolicySerialize(t *testing.T) {
	rp := &RestartPolicy{
		Mode:         RestartPolicyAlways,
		Exponential:  false,
		InitialDelay: 1 * time.Second,
		MaxDelay:     2 * time.Second,
		MaxRetries:   3,
	}
	m := rp.Serialize()
	if m["mode"] != RestartPolicyAlways || m["maxRetries"] != 3 {
		t.Errorf("RestartPolicy.Serialize failed: %v", m)
	}
}

func TestAdminEntrySerialize(t *testing.T) {
	ae := &AdminEntry{Host: "localhost", Port: 8080}
	m := ae.Serialize()
	if m["host"] != "localhost" || m["port"] != 8080 {
		t.Errorf("AdminEntry.Serialize failed: %v", m)
	}
	ae = &AdminEntry{Unix: "/tmp/socket"}
	m = ae.Serialize()
	if m["unix"] != "/tmp/socket" {
		t.Errorf("AdminEntry.Serialize unix failed: %v", m)
	}
}

func TestConfigSerialize(t *testing.T) {
	cfg := &Config{
		DefaultInitialDelay: 1 * time.Second,
		DefaultGracePeriod:  2 * time.Second,
		Admin:               &AdminEntry{Host: "localhost", Port: 8080},
		Services: map[string]*ServiceEntry{
			"svc": {
				Command:     []string{"ls"},
				GracePeriod: 2 * time.Second,
				Restart: &RestartPolicy{
					Mode:         RestartPolicyAlways,
					Exponential:  false,
					InitialDelay: 1 * time.Second,
				},
			},
		},
	}
	m := cfg.Serialize()
	if m["delay"] != "1s" || m["grace"] != "2s" {
		t.Errorf("Config.Serialize failed: %v", m)
	}
}
