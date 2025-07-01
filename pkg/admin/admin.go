package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/log"
	"github.com/eslym/stacker/pkg/supervisor"
	"golang.org/x/net/websocket"
)

// adminServer holds the HTTP server and supervisor reference
type adminServer struct {
	sup     supervisor.Supervisor
	config  *config.AdminEntry
	server  *http.Server
	ctx     context.Context
	cancel  context.CancelFunc
	verbose bool
}

type Server interface {
	Start() error
	Stop() error
}

// NewAdminServer creates a new admin HTTP server
// Now accepts a verbose parameter for configurability
func NewAdminServer(ctx context.Context, sup supervisor.Supervisor, adminCfg *config.AdminEntry, verbose ...bool) Server {
	ctx, cancel := context.WithCancel(ctx)
	v := true
	if len(verbose) > 0 {
		v = verbose[0]
	}
	return &adminServer{
		sup:     sup,
		config:  adminCfg,
		ctx:     ctx,
		cancel:  cancel,
		verbose: v,
	}
}

// registerHandlers registers all HTTP handlers for the admin server
func (a *adminServer) registerHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/services", a.handleServices)
	mux.HandleFunc("/service/", a.handleService)
	mux.HandleFunc("/process/", a.handleProcess) // New: process-level API
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if a.verbose {
			log.Printf("admin", "healthz endpoint hit from %s", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/ws/service/", websocket.Handler(a.handleServiceLogsWS))
	mux.Handle("/ws/process/", websocket.Handler(a.handleProcessLogsWS)) // New: process-level logs
}

// Start launches the HTTP admin server (goroutine)
func (a *adminServer) Start() error {
	mux := http.NewServeMux()
	a.registerHandlers(mux)

	a.server = &http.Server{
		Handler: mux,
	}

	addr := ""
	if a.config.Unix != "" {
		ln, err := net.Listen("unix", a.config.Unix)
		if err != nil {
			return err
		}
		go func() {
			_ = a.server.Serve(ln)
		}()
		addr = "unix:" + a.config.Unix
	} else {
		host := a.config.Host
		if host == "" {
			host = "localhost"
		}
		port := a.config.Port
		if port == 0 {
			port = 8080
		}
		addr = net.JoinHostPort(host, strconv.Itoa(port))
		a.server.Addr = addr
		go func() {
			_ = a.server.ListenAndServe()
		}()
	}
	if a.verbose {
		log.Printf("admin", "Admin HTTP server started at %s", addr)
	} else {
		fmt.Printf("Admin HTTP server started at %s\n", addr)
	}

	go func() {
		<-a.ctx.Done()
		if a.verbose {
			log.Printf("admin", "Shutting down admin HTTP server")
		}
		_ = a.server.Close()
	}()

	return nil
}

// Stop gracefully shuts down the admin server
func (a *adminServer) Stop() error {
	if a.verbose {
		log.Printf("admin", "Stop called on admin HTTP server")
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.server != nil {
		return a.server.Close()
	}
	return nil
}

// handleServices returns all services and their stats
func (a *adminServer) handleServices(w http.ResponseWriter, r *http.Request) {
	if a.verbose {
		log.Printf("admin", "GET /services from %s", r.RemoteAddr)
	}
	services := a.sup.GetAllServices()
	stats := make(map[string]any)
	for name, svc := range services {
		stats[name] = svc.GetSerializedStats()
	}
	writeJSON(w, stats)
}

// handleService handles actions for a single service
func (a *adminServer) handleService(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/service/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "missing service name", http.StatusBadRequest)
		if a.verbose {
			log.Printf("admin", "Bad request to /service/ (missing name) from %s", r.RemoteAddr)
		}
		return
	}
	name := parts[0]
	svc, ok := a.sup.GetService(name)
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		if a.verbose {
			log.Printf("admin", "Service %s not found for %s", name, r.RemoteAddr)
		}
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		if a.verbose {
			log.Printf("admin", "GET /service/%s from %s", name, r.RemoteAddr)
		}
		writeJSON(w, svc.GetSerializedStats())
		return
	}

	if len(parts) == 2 && r.Method == http.MethodPost {
		action := parts[1]
		var err error
		if a.verbose {
			log.Printf("admin", "POST /service/%s/%s from %s", name, action, r.RemoteAddr)
		}
		switch action {
		case "start":
			err = a.sup.StartService(name)
		case "stop":
			err = a.sup.StopService(name)
		case "restart":
			err = a.sup.RestartService(name)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			if a.verbose {
				log.Printf("admin", "Unknown action %s for service %s from %s", action, name, r.RemoteAddr)
			}
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			if a.verbose {
				log.Printf("admin", "Error performing %s on %s: %v", action, name, err)
			}
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		return
	}

	// Add resource usage endpoint: /service/{name}/resource
	if len(parts) == 2 && parts[1] == "resource" && r.Method == http.MethodGet {
		cpuSum := 0.0
		memSum := uint64(0)
		memPercentSum := float32(0)
		count := 0
		for _, proc := range svc.ListProcesses() {
			if stats, err := proc.GetResourceStats(); err == nil {
				cpuSum += stats.CPUPercent
				memSum += stats.MemoryRSS
				memPercentSum += stats.MemoryPercent
				count++
			}
		}
		writeJSON(w, map[string]any{
			"resource": map[string]any{
				"cpu_percent":    cpuSum,
				"memory_rss":     memSum,
				"memory_percent": memPercentSum,
				"process_count":  count,
			},
		})
		return
	}

	http.Error(w, "invalid request", http.StatusBadRequest)
	if a.verbose {
		log.Printf("admin", "Invalid request to /service/%s from %s", name, r.RemoteAddr)
	}
}

// handleProcess returns process info and resource stats by process ID
func (a *adminServer) handleProcess(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/process/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "missing process id", http.StatusBadRequest)
		return
	}
	pid := parts[0]
	// Search all services for this process
	for _, svc := range a.sup.GetAllServices() {
		if proc, ok := svc.GetProcessByID(pid); ok {
			stats := map[string]any{
				"id":      proc.GetID(),
				"pid":     proc.GetPid(),
				"path":    proc.GetPath(),
				"args":    proc.GetArgs(),
				"workDir": proc.GetWorkDir(),
				"env":     proc.GetEnv(),
				"running": proc.IsRunning(),
			}
			if res, err := proc.GetResourceStats(); err == nil {
				stats["resource"] = res
			}
			writeJSON(w, stats)
			return
		}
	}
	http.Error(w, "process not found", http.StatusNotFound)
}

// handleServiceLogsWS streams live logs for a service over WebSocket
func (a *adminServer) handleServiceLogsWS(ws *websocket.Conn) {
	if a.verbose {
		log.Printf("admin", "WebSocket logs connection from %s to %s", ws.Request().RemoteAddr, ws.Request().URL.Path)
	}
	defer func(ws *websocket.Conn) {
		if a.verbose {
			log.Printf("admin", "WebSocket logs connection closed for %s", ws.Request().RemoteAddr)
		}
		_ = ws.Close()
	}(ws)
	parts := strings.Split(strings.TrimPrefix(ws.Request().URL.Path, "/ws/service/"), "/")
	if len(parts) < 2 || parts[1] != "logs" {
		_ = websocket.Message.Send(ws, "invalid path")
		ws.Close() // Ensure connection is closed after error
		if a.verbose {
			log.Printf("admin", "Invalid WebSocket logs path: %s", ws.Request().URL.Path)
		}
		return
	}
	name := parts[0]
	svc, ok := a.sup.GetService(name)
	if !ok {
		_ = websocket.Message.Send(ws, "service not found")
		ws.Close() // Ensure connection is closed after error
		if a.verbose {
			log.Printf("admin", "WebSocket logs: service %s not found", name)
		}
		return
	}

	stdout := make(chan string, 100)
	stderr := make(chan string, 100)
	svc.OnStdout(stdout)
	svc.OnStderr(stderr)
	defer svc.OffStdout(stdout)
	defer svc.OffStderr(stderr)

	type logMsg struct {
		Stream string `json:"stream"`
		Line   string `json:"line"`
	}

	for {
		select {
		case line, ok := <-stdout:
			if !ok {
				return
			}
			msg, _ := json.Marshal(logMsg{Stream: "stdout", Line: line})
			if err := websocket.Message.Send(ws, string(msg)); err != nil {
				if a.verbose {
					log.Printf("admin", "WebSocket send error (stdout): %v", err)
				}
				return
			}
		case line, ok := <-stderr:
			if !ok {
				return
			}
			msg, _ := json.Marshal(logMsg{Stream: "stderr", Line: line})
			if err := websocket.Message.Send(ws, string(msg)); err != nil {
				if a.verbose {
					log.Printf("admin", "WebSocket send error (stderr): %v", err)
				}
				return
			}
		}
	}
}

// handleProcessLogsWS streams logs for a specific process by ID
func (a *adminServer) handleProcessLogsWS(ws *websocket.Conn) {
	parts := strings.Split(strings.TrimPrefix(ws.Request().URL.Path, "/ws/process/"), "/")
	if len(parts) < 2 || parts[1] != "logs" {
		_ = websocket.Message.Send(ws, "invalid path")
		ws.Close()
		return
	}
	pid := parts[0]
	var proc supervisor.Process
	for _, svc := range a.sup.GetAllServices() {
		if p, ok := svc.GetProcessByID(pid); ok {
			proc = p
			break
		}
	}
	if proc == nil {
		_ = websocket.Message.Send(ws, "process not found")
		ws.Close()
		return
	}
	stdout := make(chan string, 100)
	stderr := make(chan string, 100)
	proc.OnStdout(stdout)
	proc.OnStderr(stderr)
	defer proc.OffStdout(stdout)
	defer proc.OffStderr(stderr)
	type logMsg struct {
		Stream string `json:"stream"`
		Line   string `json:"line"`
	}
	for {
		select {
		case line, ok := <-stdout:
			if !ok {
				return
			}
			msg, _ := json.Marshal(logMsg{Stream: "stdout", Line: line})
			if err := websocket.Message.Send(ws, string(msg)); err != nil {
				return
			}
		case line, ok := <-stderr:
			if !ok {
				return
			}
			msg, _ := json.Marshal(logMsg{Stream: "stderr", Line: line})
			if err := websocket.Message.Send(ws, string(msg)); err != nil {
				return
			}
		}
	}
}

// writeJSON writes JSON response
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
