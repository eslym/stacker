package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/supervisor"
)

// Create a mock supervisor that implements the same methods as the real supervisor
type mockSupervisor struct {
	services map[string]*supervisor.ServiceInfo
	errors   map[string]error
}


// Override methods to return mock data
func (m *mockSupervisor) GetServiceStatus(name string) (*supervisor.ServiceInfo, error) {
	if err, ok := m.errors[name]; ok {
		return nil, err
	}
	if info, ok := m.services[name]; ok {
		return info, nil
	}
	return nil, errors.New("service not found")
}

func (m *mockSupervisor) GetAllServiceStatuses() map[string]*supervisor.ServiceInfo {
	return m.services
}

func (m *mockSupervisor) StartService(name string) error {
	if err, ok := m.errors[name+"_start"]; ok {
		return err
	}
	if info, ok := m.services[name]; ok {
		info.Status = supervisor.StatusRunning
	}
	return nil
}

func (m *mockSupervisor) StopService(name string) error {
	if err, ok := m.errors[name+"_stop"]; ok {
		return err
	}
	if info, ok := m.services[name]; ok {
		// For cron jobs, set RunningProcesses to 0 but keep status as is
		if info.Config.Cron != "" {
			info.RunningProcesses = 0
			info.Status = supervisor.StatusStopped
		} else {
			// For regular services, just set status to stopped
			info.Status = supervisor.StatusStopped
		}
	}
	return nil
}

func (m *mockSupervisor) RestartService(name string) error {
	if err, ok := m.errors[name+"_restart"]; ok {
		return err
	}
	if info, ok := m.services[name]; ok {
		info.Status = supervisor.StatusRunning
	}
	return nil
}

func (m *mockSupervisor) EnableCronJob(name string) error {
	if err, ok := m.errors[name+"_enable"]; ok {
		return err
	}
	if info, ok := m.services[name]; ok {
		info.Status = supervisor.StatusScheduled
	}
	return nil
}

func (m *mockSupervisor) DisableCronJob(name string) error {
	if err, ok := m.errors[name+"_disable"]; ok {
		return err
	}
	if info, ok := m.services[name]; ok {
		info.Status = supervisor.StatusStopped
	}
	return nil
}

// Create a new mock supervisor
func newMockSupervisor() *mockSupervisor {
	return &mockSupervisor{
		services: make(map[string]*supervisor.ServiceInfo),
		errors:   make(map[string]error),
	}
}

// Create a mock config for testing
func mockConfig() *config.Config {
	return &config.Config{
		Restart: "1s",
		Grace:   "1s",
		Admin: map[string]interface{}{
			"host": "localhost",
			"port": 8080,
		},
		Services: map[string]config.Process{
			"service1": {
				Cmd:     []interface{}{"echo", "service1"},
				Restart: true,
			},
			"service2": {
				Cmd:     []interface{}{"echo", "service2"},
				Restart: false,
			},
			"cron-service": {
				Cmd:  []interface{}{"echo", "cron-service"},
				Cron: "* * * * *",
			},
		},
	}
}

func TestHandleServices(t *testing.T) {
	// Create mock supervisor
	sup := newMockSupervisor()
	sup.services["service1"] = &supervisor.ServiceInfo{
		Name:   "service1",
		Status: supervisor.StatusRunning,
	}
	sup.services["service2"] = &supervisor.ServiceInfo{
		Name:   "service2",
		Status: supervisor.StatusStopped,
	}

	// Create admin server
	cfg := mockConfig()
	adminServer := NewAdminServer(cfg, sup)

	// Create request
	req, err := http.NewRequest("GET", "/api/services", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	adminServer.handleServices(rr, req)

	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check content type
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Handler returned wrong content type: got %v want %v", contentType, "application/json")
	}

	// Parse response
	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check response
	if len(response) != 2 {
		t.Errorf("Expected 2 services, got %d", len(response))
	}
	if _, ok := response["service1"]; !ok {
		t.Errorf("Expected service1 to exist in response")
	}
	if _, ok := response["service2"]; !ok {
		t.Errorf("Expected service2 to exist in response")
	}
}

func TestHandleService_Get(t *testing.T) {
	// Create mock supervisor
	sup := newMockSupervisor()
	sup.services["service1"] = &supervisor.ServiceInfo{
		Name:   "service1",
		Status: supervisor.StatusRunning,
	}

	// Create admin server
	cfg := mockConfig()
	adminServer := NewAdminServer(cfg, sup)

	// Create request
	req, err := http.NewRequest("GET", "/api/services/service1", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	adminServer.handleService(rr, req)

	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check content type
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Handler returned wrong content type: got %v want %v", contentType, "application/json")
	}

	// Parse response
	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check response
	if name, ok := response["name"]; !ok || name != "service1" {
		t.Errorf("Expected name to be service1, got %v", name)
	}
	if status, ok := response["status"]; !ok || status != string(supervisor.StatusRunning) {
		t.Errorf("Expected status to be %s, got %v", supervisor.StatusRunning, status)
	}
}

func TestHandleService_Get_Unknown(t *testing.T) {
	// Create a real supervisor with a minimal configuration
	sup := createTestSupervisor()
	defer sup.Stop()

	// Create admin server
	cfg := createTestConfig()
	adminServer := NewAdminServer(cfg, sup)

	// Create request
	req, err := http.NewRequest("GET", "/api/services/unknown", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	adminServer.handleService(rr, req)

	// Check status code
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestHandleService_Post_Start(t *testing.T) {
	// Create mock supervisor
	sup := newMockSupervisor()
	sup.services["service1"] = &supervisor.ServiceInfo{
		Name:   "service1",
		Status: supervisor.StatusStopped,
	}

	// Create admin server
	cfg := mockConfig()
	adminServer := NewAdminServer(cfg, sup)

	// Create request body
	body := map[string]string{
		"action": "start",
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	// Create request
	req, err := http.NewRequest("POST", "/api/services/service1", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler
	adminServer.handleService(rr, req)

	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check service status
	if sup.services["service1"].Status != supervisor.StatusRunning {
		t.Errorf("Expected service status to be running, got %s", sup.services["service1"].Status)
	}
}

func TestHandleService_Post_StopCronJob(t *testing.T) {
	// Create mock supervisor
	sup := newMockSupervisor()
	sup.services["cron-service"] = &supervisor.ServiceInfo{
		Name:             "cron-service",
		Status:           supervisor.StatusScheduled,
		RunningProcesses: 2, // Simulate 2 running processes
		Config: config.Process{
			Cmd:  []interface{}{"echo", "cron-service"},
			Cron: "* * * * *",
		},
	}

	// Create admin server
	cfg := mockConfig()
	adminServer := NewAdminServer(cfg, sup)

	// Test stop action
	body := map[string]string{
		"action": "stop",
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("POST", "/api/services/cron-service", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	adminServer.handleService(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code for stop: got %v want %v", status, http.StatusOK)
	}

	// Check that RunningProcesses is 0 after stopping
	if sup.services["cron-service"].RunningProcesses != 0 {
		t.Errorf("Expected RunningProcesses to be 0 after stopping, got %d", sup.services["cron-service"].RunningProcesses)
	}

	// Check that the cron job is still scheduled (not stopped)
	if sup.services["cron-service"].Status != supervisor.StatusStopped {
		t.Errorf("Expected service status to be stopped after stopping processes, got %s", sup.services["cron-service"].Status)
	}
}

func TestHandleService_Post_EnableDisable(t *testing.T) {
	// Create mock supervisor
	sup := newMockSupervisor()
	sup.services["cron-service"] = &supervisor.ServiceInfo{
		Name:   "cron-service",
		Status: supervisor.StatusStopped,
		Config: config.Process{
			Cmd:  []interface{}{"echo", "cron-service"},
			Cron: "* * * * *",
		},
	}

	// Create admin server
	cfg := mockConfig()
	adminServer := NewAdminServer(cfg, sup)

	// Test enable action
	body := map[string]string{
		"action": "enable",
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("POST", "/api/services/cron-service", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	adminServer.handleService(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code for enable: got %v want %v", status, http.StatusOK)
	}

	if sup.services["cron-service"].Status != supervisor.StatusScheduled {
		t.Errorf("Expected service status to be scheduled after enable, got %s", sup.services["cron-service"].Status)
	}

	// Test disable action
	body = map[string]string{
		"action": "disable",
	}
	bodyBytes, err = json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	req, err = http.NewRequest("POST", "/api/services/cron-service", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr = httptest.NewRecorder()
	adminServer.handleService(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code for disable: got %v want %v", status, http.StatusOK)
	}

	if sup.services["cron-service"].Status != supervisor.StatusStopped {
		t.Errorf("Expected service status to be stopped after disable, got %s", sup.services["cron-service"].Status)
	}
}

func TestFormatServiceInfo(t *testing.T) {
	// Create admin server
	cfg := mockConfig()
	sup := newMockSupervisor()
	adminServer := NewAdminServer(cfg, sup)

	// Create service info
	info := &supervisor.ServiceInfo{
		Name:             "service1",
		Status:           supervisor.StatusRunning,
		Pid:              1234,
		StartTime:        time.Now(),
		Uptime:           time.Hour,
		RestartCount:     2,
		ExitCode:         0,
		CpuPercent:       10.5,
		MemoryUsage:      1024 * 1024 * 10, // 10 MB
		LastUpdated:      time.Now(),
		RunningProcesses: 1,
		Config: config.Process{
			Cmd:     []interface{}{"echo", "service1"},
			Restart: true,
		},
	}

	// Format service info
	result := adminServer.formatServiceInfo(info)

	// Check result
	if result["name"] != "service1" {
		t.Errorf("Expected name to be service1, got %v", result["name"])
	}
	if result["status"] != string(supervisor.StatusRunning) {
		t.Errorf("Expected status to be %s, got %v", supervisor.StatusRunning, result["status"])
	}
	if result["pid"] != 1234 {
		t.Errorf("Expected pid to be 1234, got %v", result["pid"])
	}
	if result["restartCount"] != 2 {
		t.Errorf("Expected restartCount to be 2, got %v", result["restartCount"])
	}
	if result["exitCode"] != 0 {
		t.Errorf("Expected exitCode to be 0, got %v", result["exitCode"])
	}
	if result["runningProcesses"] != 1 {
		t.Errorf("Expected runningProcesses to be 1, got %v", result["runningProcesses"])
	}

	// Check resource usage
	resourceUsage, ok := result["resourceUsage"].(map[string]interface{})
	if !ok {
		t.Errorf("Expected resourceUsage to be a map, got %T", result["resourceUsage"])
	} else {
		if resourceUsage["cpuPercent"] != 10.5 {
			t.Errorf("Expected cpuPercent to be 10.5, got %v", resourceUsage["cpuPercent"])
		}
	}

	// Check actions
	actions, ok := result["actions"].(map[string]interface{})
	if !ok {
		t.Errorf("Expected actions to be a map, got %T", result["actions"])
	} else {
		if len(actions) != 3 {
			t.Errorf("Expected 3 actions, got %d", len(actions))
		}
		if _, ok := actions["start"]; !ok {
			t.Errorf("Expected start action to exist")
		}
		if _, ok := actions["stop"]; !ok {
			t.Errorf("Expected stop action to exist")
		}
		if _, ok := actions["restart"]; !ok {
			t.Errorf("Expected restart action to exist")
		}
	}
}
