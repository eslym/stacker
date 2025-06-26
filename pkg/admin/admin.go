package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/supervisor"
)

// ClientInfo represents information about a connected client
type ClientInfo struct {
	Channel chan []byte
	Filter  string
}

// SupervisorInterface defines the methods required by the AdminServer
type SupervisorInterface interface {
	GetServiceStatus(name string) (*supervisor.ServiceInfo, error)
	GetAllServiceStatuses() map[string]*supervisor.ServiceInfo
	StartService(name string) error
	StopService(name string) error
	RestartService(name string) error
	EnableCronJob(name string) error
	DisableCronJob(name string) error
}

// AdminServer represents the HTTP admin interface
type AdminServer struct {
	supervisor SupervisorInterface
	config     *config.Config
	server     *http.Server
	clients    map[string]*ClientInfo
	mu         sync.RWMutex
}

// NewAdminServer creates a new admin server
func NewAdminServer(cfg *config.Config, sup SupervisorInterface) *AdminServer {
	return &AdminServer{
		supervisor: sup,
		config:     cfg,
		clients:    make(map[string]*ClientInfo),
	}
}

// Start starts the admin server
func (a *AdminServer) Start() error {
	if a.config.Admin == nil {
		return nil
	}

	// Create router
	mux := http.NewServeMux()

	// Register endpoints
	mux.HandleFunc("/api/services", a.handleServices)
	mux.HandleFunc("/api/services/", a.handleService)
	mux.HandleFunc("/api/logs", a.handleLogs)

	// Create server
	a.server = &http.Server{
		Handler: mux,
	}

	// Start server
	go func() {
		var err error

		switch admin := a.config.Admin.(type) {
		case map[string]interface{}:
			// Check for host/port configuration
			if host, ok := admin["host"].(string); ok {
				if port, ok := admin["port"].(float64); ok {
					addr := fmt.Sprintf("%s:%d", host, int(port))
					log.Printf("Starting admin server on %s", addr)
					a.server.Addr = addr
					err = a.server.ListenAndServe()
				}
			} else if sock, ok := admin["sock"].(string); ok {
				// Unix socket (Linux only)
				log.Printf("Starting admin server on unix socket %s", sock)
				listener, listenErr := net.Listen("unix", sock)
				if listenErr != nil {
					log.Printf("Failed to listen on unix socket: %v", listenErr)
					return
				}
				err = a.server.Serve(listener)
			}
		}

		if err != nil && err != http.ErrServerClosed {
			log.Printf("Admin server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the admin server
func (a *AdminServer) Stop() error {
	if a.server == nil {
		return nil
	}

	// Close all client connections
	a.mu.Lock()
	for id, clientInfo := range a.clients {
		close(clientInfo.Channel)
		delete(a.clients, id)
	}
	a.mu.Unlock()

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown the server gracefully
	err := a.server.Shutdown(ctx)
	if err != nil {
		// If shutdown times out or fails, force close
		log.Printf("Admin server graceful shutdown failed: %v, forcing close", err)
		return a.server.Close()
	}

	return nil
}

// handleServices handles requests to /api/services
func (a *AdminServer) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get all service statuses
		statuses := a.supervisor.GetAllServiceStatuses()

		// Convert to JSON-friendly format
		result := make(map[string]interface{})
		for name, info := range statuses {
			result[name] = a.formatServiceInfo(info)
		}

		a.respondJSON(w, result)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleService handles requests to /api/services/{name}
func (a *AdminServer) handleService(w http.ResponseWriter, r *http.Request) {
	// Extract service name from URL
	name := r.URL.Path[len("/api/services/"):]
	if name == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get service status
		info, err := a.supervisor.GetServiceStatus(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		a.respondJSON(w, a.formatServiceInfo(info))
	case http.MethodPost:
		// Parse action
		var action struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Get service info to check if it's a cron job
		info, err := a.supervisor.GetServiceStatus(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Check if it's a cron job
		isCronJob := info.Config.Cron != ""

		// Handle action based on service type
		switch action.Action {
		case "start":
			if isCronJob {
				err = fmt.Errorf("start is not applicable for cron job %s, use enable instead", name)
			} else {
				err = a.supervisor.StartService(name)
			}
		case "stop":
			// Stop is applicable to both regular services and cron jobs
			err = a.supervisor.StopService(name)
		case "restart":
			if isCronJob {
				err = fmt.Errorf("restart is not applicable for cron job %s", name)
			} else {
				err = a.supervisor.RestartService(name)
			}
		case "enable":
			if isCronJob {
				err = a.supervisor.EnableCronJob(name)
			} else {
				err = fmt.Errorf("enable is not applicable for regular service %s, use start instead", name)
			}
		case "disable":
			if isCronJob {
				err = a.supervisor.DisableCronJob(name)
			} else {
				err = fmt.Errorf("disable is not applicable for regular service %s, use stop instead", name)
			}
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Get updated status
		info, err = a.supervisor.GetServiceStatus(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		a.respondJSON(w, a.formatServiceInfo(info))
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLogs handles requests to /api/logs
func (a *AdminServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get service filter from query parameters
	serviceFilter := r.URL.Query().Get("service")

	// Create a channel for this client
	clientID := fmt.Sprintf("%d", time.Now().UnixNano())
	messageCh := make(chan []byte, 100)

	// Create client info
	clientInfo := &ClientInfo{
		Channel: messageCh,
		Filter:  serviceFilter,
	}

	// Register client
	a.mu.Lock()
	a.clients[clientID] = clientInfo
	a.mu.Unlock()

	// Clean up when the client disconnects
	defer func() {
		a.mu.Lock()
		delete(a.clients, clientID)
		close(messageCh)
		a.mu.Unlock()
	}()

	// Send messages to client
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Log connection
	log.Printf("Client connected to log stream with filter: %s", serviceFilter)

	// Keep the connection open
	for {
		select {
		case msg, ok := <-clientInfo.Channel:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// BroadcastLog broadcasts a log message to all connected clients
func (a *AdminServer) BroadcastLog(service, message string) {
	// Create log message
	logMsg := struct {
		Timestamp string `json:"timestamp"`
		Service   string `json:"service"`
		Message   string `json:"message"`
	}{
		Timestamp: time.Now().Format(time.RFC3339),
		Service:   service,
		Message:   message,
	}

	// Convert to JSON
	data, err := json.Marshal(logMsg)
	if err != nil {
		log.Printf("Failed to marshal log message: %v", err)
		return
	}

	// Send to all clients
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, clientInfo := range a.clients {
		// Apply service filter if specified
		if clientInfo.Filter != "" && clientInfo.Filter != service {
			continue
		}

		select {
		case clientInfo.Channel <- data:
			// Message sent
		default:
			// Channel full, skip
		}
	}
}

// formatServiceInfo formats service info for JSON response
func (a *AdminServer) formatServiceInfo(info *supervisor.ServiceInfo) map[string]interface{} {
	result := map[string]interface{}{
		"name":             info.Name,
		"status":           string(info.Status),
		"pid":              info.Pid,
		"restartCount":     info.RestartCount,
		"exitCode":         info.ExitCode,
		"runningProcesses": info.RunningProcesses,
	}

	if !info.StartTime.IsZero() {
		result["startTime"] = info.StartTime.Format(time.RFC3339)
		result["uptime"] = info.Uptime.String()
	}

	if !info.NextRestart.IsZero() {
		result["nextRestart"] = info.NextRestart.Format(time.RFC3339)
	}

	if !info.NextRun.IsZero() {
		result["nextRun"] = info.NextRun.Format(time.RFC3339)
	}

	if info.Error != "" {
		result["error"] = info.Error
	}

	// Add resource usage information
	if info.Status == supervisor.StatusRunning && !info.LastUpdated.IsZero() {
		result["resourceUsage"] = map[string]interface{}{
			"cpuPercent":  info.CpuPercent,
			"memoryUsage": formatBytes(info.MemoryUsage),
			"lastUpdated": info.LastUpdated.Format(time.RFC3339),
		}
	}

	// Add available actions based on service type
	isCronJob := info.Config.Cron != ""
	actions := make(map[string]interface{})

	if isCronJob {
		// Cron job actions
		actions["enable"] = map[string]interface{}{
			"description": "Enable the cron job",
			"applicable":  info.Status != supervisor.StatusScheduled,
		}
		actions["disable"] = map[string]interface{}{
			"description": "Disable the cron job without stopping processes",
			"applicable":  info.Status == supervisor.StatusScheduled,
		}
		actions["stop"] = map[string]interface{}{
			"description": "Stop all running processes",
			"applicable":  info.RunningProcesses > 0,
		}
	} else {
		// Regular service actions
		actions["start"] = map[string]interface{}{
			"description": "Start the service",
			"applicable":  info.Status != supervisor.StatusRunning,
		}
		actions["stop"] = map[string]interface{}{
			"description": "Stop the service",
			"applicable":  info.Status == supervisor.StatusRunning,
		}
		actions["restart"] = map[string]interface{}{
			"description": "Restart the service",
			"applicable":  true,
		}
	}

	result["actions"] = actions
	result["isCronJob"] = isCronJob

	return result
}

// formatBytes formats bytes to a human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// respondJSON sends a JSON response
func (a *AdminServer) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
