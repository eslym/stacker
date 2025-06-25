package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/supervisor"
)

// createTestConfig creates a minimal configuration for testing
func createTestConfig() *config.Config {
	return &config.Config{
		Restart: "1s",
		Grace:   "1s",
		Admin: map[string]interface{}{
			"host": "localhost",
			"port": 8080,
		},
		Services: map[string]config.Process{
			"test-service": {
				Cmd:     []interface{}{"echo", "test"},
				Restart: true,
			},
		},
	}
}

// createTestSupervisor creates a supervisor with a minimal configuration
func createTestSupervisor() *supervisor.Supervisor {
	cfg := createTestConfig()
	activeServices := map[string]bool{
		"test-service": true,
	}
	sup := supervisor.NewSupervisor(cfg, activeServices, false)
	err := sup.Start()
	if err != nil {
		panic(err)
	}
	return sup
}

// TestAdminServer_Integration tests the admin server with a real supervisor
func TestAdminServer_Integration(t *testing.T) {
	// Create supervisor
	sup := createTestSupervisor()
	// Ensure supervisor is stopped after the test
	defer sup.Stop()

	// Create admin server
	cfg := createTestConfig()
	adminServer := NewAdminServer(cfg, sup)

	// Test handleServices
	t.Run("HandleServices", func(t *testing.T) {
		// Create request
		req := httptest.NewRequest("GET", "/api/services", nil)
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
		if len(response) != 1 {
			t.Errorf("Expected 1 service, got %d", len(response))
		}
		if _, ok := response["test-service"]; !ok {
			t.Errorf("Expected test-service to exist in response")
		}
	})

	// Test handleService GET
	t.Run("HandleService_Get", func(t *testing.T) {
		// Create request
		req := httptest.NewRequest("GET", "/api/services/test-service", nil)
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
		if name, ok := response["name"]; !ok || name != "test-service" {
			t.Errorf("Expected name to be test-service, got %v", name)
		}
	})

	// Test handleService GET unknown
	t.Run("HandleService_Get_Unknown", func(t *testing.T) {
		// Create request
		req := httptest.NewRequest("GET", "/api/services/unknown", nil)
		rr := httptest.NewRecorder()

		// Call handler
		adminServer.handleService(rr, req)

		// Check status code
		if status := rr.Code; status != http.StatusNotFound {
			t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
		}
	})

	// Test formatServiceInfo
	t.Run("FormatServiceInfo", func(t *testing.T) {
		// Create service info
		info := &supervisor.ServiceInfo{
			Name:         "test-service",
			Status:       supervisor.StatusRunning,
			Pid:          1234,
			StartTime:    time.Now(),
			Uptime:       time.Hour,
			RestartCount: 2,
			ExitCode:     0,
			CpuPercent:   10.5,
			MemoryUsage:  1024 * 1024 * 10, // 10 MB
			LastUpdated:  time.Now(),
			Config: config.Process{
				Cmd:     []interface{}{"echo", "test"},
				Restart: true,
			},
		}

		// Format service info
		result := adminServer.formatServiceInfo(info)

		// Check result
		if result["name"] != "test-service" {
			t.Errorf("Expected name to be test-service, got %v", result["name"])
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
	})
}
