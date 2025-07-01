package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/websocket"
)

type mockSupervisor struct {
	supervisor.Supervisor
	services map[string]*mockService
}

type mockService struct {
	name  string
	stats map[string]interface{}
}

func (m *mockSupervisor) GetAllServices() map[string]supervisor.Service {
	svcs := make(map[string]supervisor.Service)
	for k, v := range m.services {
		svcs[k] = v
	}
	return svcs
}

func (m *mockSupervisor) GetService(name string) (supervisor.Service, bool) {
	svc, ok := m.services[name]
	return svc, ok
}

func (m *mockService) GetName() string                      { return m.name }
func (m *mockService) GetType() supervisor.ServiceType      { return 0 }
func (m *mockService) AsProcess() supervisor.ProcessService { return nil }
func (m *mockService) AsCron() supervisor.CronService       { return nil }
func (m *mockService) OnStdout(listener chan string) {
	go func() {
		listener <- "stdout test log"
		close(listener)
	}()
}
func (m *mockService) OnStderr(listener chan string) {
	go func() {
		listener <- "stderr test log"
		close(listener)
	}()
}
func (m *mockService) OffStdout(listener chan string)     {}
func (m *mockService) OffStderr(listener chan string)     {}
func (m *mockService) GetSerializedStats() map[string]any { return m.stats }

func (m *mockSupervisor) StartService(name string) error   { return nil }
func (m *mockSupervisor) StopService(name string) error    { return nil }
func (m *mockSupervisor) RestartService(name string) error { return nil }

func TestAdminServer_Healthz(t *testing.T) {
	ms := &mockSupervisor{}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(body))
}

func TestAdminServer_ServicesEndpoint(t *testing.T) {
	ms := &mockSupervisor{
		services: map[string]*mockService{
			"svc1": {name: "svc1", stats: map[string]interface{}{"status": "running"}},
			"svc2": {name: "svc2", stats: map[string]interface{}{"status": "stopped"}},
		},
	}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/services")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	_ = json.Unmarshal(body, &data)
	assert.Contains(t, data, "svc1")
	assert.Contains(t, data, "svc2")
}

func TestAdminServer_ServiceNotFound(t *testing.T) {
	ms := &mockSupervisor{services: map[string]*mockService{}}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/service/unknown")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAdminServer_ServiceStats(t *testing.T) {
	ms := &mockSupervisor{
		services: map[string]*mockService{
			"svc1": {name: "svc1", stats: map[string]interface{}{"status": "running"}},
		},
	}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/service/svc1")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	_ = json.Unmarshal(body, &data)
	assert.Equal(t, "running", data["status"])
}

func TestAdminServer_ServiceActions(t *testing.T) {
	ms := &mockSupervisor{
		services: map[string]*mockService{
			"svc1": {name: "svc1", stats: map[string]interface{}{"status": "running"}},
		},
	}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	for _, action := range []string{"start", "stop", "restart"} {
		resp, err := http.Post(ts.URL+"/service/svc1/"+action, "application/json", nil)
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		var data map[string]interface{}
		_ = json.Unmarshal(body, &data)
		assert.Equal(t, "ok", data["status"])
	}
}

func TestAdminServer_ServiceInvalidAction(t *testing.T) {
	ms := &mockSupervisor{
		services: map[string]*mockService{
			"svc1": {name: "svc1", stats: map[string]interface{}{"status": "running"}},
		},
	}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/service/svc1/invalid", "application/json", nil)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAdminServer_ServiceMissingName(t *testing.T) {
	ms := &mockSupervisor{}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/service/")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAdminServer_WebSocketLogs(t *testing.T) {
	ms := &mockSupervisor{
		services: map[string]*mockService{
			"svc1": {name: "svc1", stats: map[string]interface{}{"status": "running"}},
		},
	}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	origin := "http://localhost/"
	wsURL := "ws" + ts.URL[len("http"):] + "/ws/service/svc1/logs"

	ws, err := websocket.Dial(wsURL, "", origin)
	assert.NoError(t, err)
	defer ws.Close()

	type logMsg struct {
		Stream string `json:"stream"`
		Line   string `json:"line"`
	}

	var msg logMsg
	var gotStdout, gotStderr bool
	for i := 0; i < 2; i++ {
		var raw string
		err := websocket.Message.Receive(ws, &raw)
		assert.NoError(t, err)
		_ = json.Unmarshal([]byte(raw), &msg)
		if msg.Stream == "stdout" && msg.Line == "stdout test log" {
			gotStdout = true
		}
		if msg.Stream == "stderr" && msg.Line == "stderr test log" {
			gotStderr = true
		}
	}
	assert.True(t, gotStdout, "should receive stdout log")
	assert.True(t, gotStderr, "should receive stderr log")
}
