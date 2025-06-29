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
func NewAdminServer(ctx context.Context, sup supervisor.Supervisor, adminCfg *config.AdminEntry) Server {
	ctx, cancel := context.WithCancel(ctx)
	return &adminServer{
		sup:     sup,
		config:  adminCfg,
		ctx:     ctx,
		cancel:  cancel,
		verbose: true, // Set to true for verbose logging; could be configurable
	}
}

// Start launches the HTTP admin server (goroutine)
func (a *adminServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/services", a.handleServices)
	mux.HandleFunc("/service/", a.handleService)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if a.verbose {
			log.Printf("admin", "healthz endpoint hit from %s", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/ws/service/", websocket.Handler(a.handleServiceLogsWS))

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

	http.Error(w, "invalid request", http.StatusBadRequest)
	if a.verbose {
		log.Printf("admin", "Invalid request to /service/%s from %s", name, r.RemoteAddr)
	}
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
		if a.verbose {
			log.Printf("admin", "Invalid WebSocket logs path: %s", ws.Request().URL.Path)
		}
		return
	}
	name := parts[0]
	svc, ok := a.sup.GetService(name)
	if !ok {
		_ = websocket.Message.Send(ws, "service not found")
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

// writeJSON writes JSON response
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
