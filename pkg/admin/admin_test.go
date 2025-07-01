package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	name     string
	stats    map[string]interface{}
	processes []supervisor.Process
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
		// Do not close immediately; let the handler read it first
	}()
}
func (m *mockService) OnStderr(listener chan string) {
	go func() {
		listener <- "stderr test log"
		// Do not close immediately; let the handler read it first
	}()
}
func (m *mockService) OffStdout(listener chan string)      {}
func (m *mockService) OffStderr(listener chan string)      {}
func (m *mockService) GetSerializedStats() map[string]any  { return m.stats }
func (m *mockService) ListProcesses() []supervisor.Process { return m.processes }
func (m *mockService) GetProcessByID(id string) (supervisor.Process, bool) {
	for _, p := range m.processes {
		if p.GetID() == id {
			return p, true
		}
	}
	return nil, false
}

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

func TestAdminServer_ProcessesEndpoint(t *testing.T) {
	proc1 := &mockProcess{id: "p1", pid: 101, running: true, resource: &supervisor.ProcessResourceStats{CPUPercent: 1.1}}
	proc2 := &mockProcess{id: "p2", pid: 102, running: false, resource: &supervisor.ProcessResourceStats{CPUPercent: 2.2}}
	ms := &mockSupervisor{
		services: map[string]*mockService{
			"svc1": {name: "svc1", processes: []supervisor.Process{proc1}},
			"svc2": {name: "svc2", processes: []supervisor.Process{proc2}},
		},
	}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/processes")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var data []map[string]interface{}
	_ = json.Unmarshal(body, &data)
	assert.Len(t, data, 2)
	ids := []string{data[0]["id"].(string), data[1]["id"].(string)}
	assert.Contains(t, ids, "p1")
	assert.Contains(t, ids, "p2")
}

func TestAdminServer_ServiceProcessesEndpoint(t *testing.T) {
	proc1 := &mockProcess{id: "p1", pid: 101, running: true, resource: &supervisor.ProcessResourceStats{CPUPercent: 1.1}}
	ms := &mockSupervisor{
		services: map[string]*mockService{
			"svc1": {name: "svc1", processes: []supervisor.Process{proc1}},
		},
	}
	adminCfg := &config.AdminEntry{Host: "127.0.0.1", Port: 0}
	server := NewAdminServer(context.Background(), ms, adminCfg, false).(*adminServer)

	mux := http.NewServeMux()
	server.registerHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/service/svc1/processes")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var data []map[string]interface{}
	_ = json.Unmarshal(body, &data)
	assert.Len(t, data, 1)
	assert.Equal(t, "p1", data[0]["id"])
}

// mockProcess implements supervisor.Process for testing
type mockProcess struct {
	id      string
	pid     int
	running bool
	resource *supervisor.ProcessResourceStats
}

func (m *mockProcess) GetID() string                { return m.id }
func (m *mockProcess) GetPid() int                  { return m.pid }
func (m *mockProcess) IsRunning() bool              { return m.running }
func (m *mockProcess) GetResourceStats() (*supervisor.ProcessResourceStats, error) { return m.resource, nil }
func (m *mockProcess) GetPath() string              { return "/mock/path" }
func (m *mockProcess) GetArgs() []string            { return []string{"--mock"} }
func (m *mockProcess) GetWorkDir() string           { return "/mock/dir" }
func (m *mockProcess) GetEnv() map[string]string    { return map[string]string{"MOCK": "1"} }
func (m *mockProcess) OnStdout(chan string)         {}
func (m *mockProcess) OnStderr(chan string)         {}
func (m *mockProcess) OffStdout(chan string)        {}
func (m *mockProcess) OffStderr(chan string)        {}
func (m *mockProcess) OnExit(chan int)              {}
func (m *mockProcess) OffExit(chan int)             {}
func (m *mockProcess) Start() error                 { return nil }
func (m *mockProcess) Stop(timeout time.Duration) error { return nil }
func (m *mockProcess) Kill() error                  { return nil }
