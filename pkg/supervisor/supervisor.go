package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/adhocore/gronx"
	"github.com/eslym/stacker/pkg/config"
	"github.com/eslym/stacker/pkg/log"
)

var ErrServiceNotFound = errors.New("service not found")
var ErrServiceAlreadyExists = errors.New("service already exists")

type supervisor struct {
	services       map[string]Service
	cron           *gronx.Gronx
	verbose        bool
	cronTicker     *time.Ticker
	cronTickerDone chan struct{}
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.Mutex
}

type Supervisor interface {
	GetService(name string) (Service, bool)
	AddService(name string, service Service) error
	RemoveService(name string) error
	StartService(name string) error
	StopService(name string) error
	RestartService(name string) error
	Start() error
	Stop() error
	GetAllServices() map[string]Service
}

// NewSupervisor creates a new Supervisor instance
func NewSupervisor(verbose bool) Supervisor {
	return &supervisor{
		services: make(map[string]Service),
		cron:     gronx.New(),
		verbose:  verbose,
	}
}

// CreateSupervisorFromConfig creates a new Supervisor from a config.Config
func CreateSupervisorFromConfig(cfg *config.Config, serviceSelection map[string]bool, verbose bool) (Supervisor, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	// Create a new supervisor
	s := NewSupervisor(verbose).(*supervisor)

	// Create services from config
	for name, entry := range cfg.Services {
		if !serviceSelection[name] {
			// Skip services that are not selected
			continue
		}

		var service Service
		switch entry.Type {
		case config.ServiceTypeService, "":
			// Regular service
			if entry.Cron != "" {
				// Cron service
				service = NewCronService(entry, s.cron)
			} else {
				// Process service
				service = NewProcessService(entry)
			}
		case config.ServiceTypeCron:
			// Cron service
			service = NewCronService(entry, s.cron)
		default:
			return nil, fmt.Errorf("unknown service type: %s", entry.Type)
		}

		if service == nil {
			return nil, fmt.Errorf("failed to create service: %s", name)
		}

		// Set the service name
		switch service.GetType() {
		case ServiceTypeProcess:
			service.AsProcess().(*processService).name = name
		case ServiceTypeCron:
			service.AsCron().(*cronService).name = name
		}

		// Add the service to the supervisor
		if err := s.AddService(name, service); err != nil {
			return nil, fmt.Errorf("failed to add service %s: %w", name, err)
		}
	}

	return s, nil
}

// GetService returns a service by name
func (s *supervisor) GetService(name string) (Service, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	service, ok := s.services[name]
	return service, ok
}

// AddService adds a service to the supervisor
func (s *supervisor) AddService(name string, service Service) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.services[name]; exists {
		return fmt.Errorf("%w: %s", ErrServiceAlreadyExists, name)
	}
	s.services[name] = service
	return nil
}

// RemoveService removes a service from the supervisor
func (s *supervisor) RemoveService(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.services[name]; !exists {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, name)
	}
	delete(s.services, name)
	return nil
}

// StartService starts a service
func (s *supervisor) StartService(name string) error {
	service, ok := s.GetService(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, name)
	}

	// Start the service based on its type
	switch service.GetType() {
	case ServiceTypeProcess:
		return service.AsProcess().Start()
	case ServiceTypeCron:
		service.AsCron().SetEnabled(true)
		return nil
	default:
		return fmt.Errorf("unknown service type: %d", service.GetType())
	}
}

// StopService stops a service
func (s *supervisor) StopService(name string) error {
	service, ok := s.GetService(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, name)
	}

	// Stop the service based on its type
	switch service.GetType() {
	case ServiceTypeProcess:
		return service.AsProcess().Stop(true)
	case ServiceTypeCron:
		service.AsCron().SetEnabled(false)
		return service.AsCron().StopAll()
	default:
		return fmt.Errorf("unknown service type: %d", service.GetType())
	}
}

// RestartService restarts a service
func (s *supervisor) RestartService(name string) error {
	service, ok := s.GetService(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrServiceNotFound, name)
	}

	// Restart the service based on its type
	switch service.GetType() {
	case ServiceTypeProcess:
		return service.AsProcess().Restart()
	case ServiceTypeCron:
		// Cron services can't be restarted
		return errors.New("cron services cannot be restarted")
	default:
		return fmt.Errorf("unknown service type: %d", service.GetType())
	}
}

// Start starts all services and the cron scheduler
func (s *supervisor) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if the supervisor is already running
	if s.cronTicker != nil {
		// Clean up the previous context if it exists
		if s.cancel != nil {
			s.cancel()
		}
		s.cronTicker.Stop()
		if s.cronTickerDone != nil {
			close(s.cronTickerDone)
		}
	}

	// Create a new cancelable context
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Start the cron scheduler
	s.cronTicker = time.NewTicker(time.Second)
	s.cronTickerDone = make(chan struct{})
	go s.runCronScheduler(s.ctx)

	// Start all process services
	for name, service := range s.services {
		if service.GetType() == ServiceTypeProcess {
			if err := service.AsProcess().Start(); err != nil {
				if s.verbose {
					log.Errorf("supervisor", "failed to start service %s: %s", name, err)
				}
				// Continue with other services
			}
		}
	}

	return nil
}

// Stop stops all services and the cron scheduler
func (s *supervisor) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cancel the context to signal goroutines to stop
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}

	// Stop the cron scheduler
	if s.cronTicker != nil {
		s.cronTicker.Stop()
		s.cronTicker = nil
	}

	// Close the cronTickerDone channel if it exists
	if s.cronTickerDone != nil {
		close(s.cronTickerDone)
		s.cronTickerDone = nil
	}

	// Stop all services
	for name, service := range s.services {
		switch service.GetType() {
		case ServiceTypeProcess:
			if err := service.AsProcess().Stop(true); err != nil {
				if s.verbose {
					log.Errorf("supervisor", "failed to stop service %s: %s", name, err)
				}
				// Continue with other services
			}
		case ServiceTypeCron:
			if err := service.AsCron().StopAll(); err != nil {
				if s.verbose {
					log.Errorf("supervisor", "failed to stop cron service %s: %s", name, err)
				}
				// Continue with other services
			}
		}
	}

	return nil
}

// GetAllServices returns all services
func (s *supervisor) GetAllServices() map[string]Service {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a copy of the services map
	services := make(map[string]Service)
	for name, service := range s.services {
		services[name] = service
	}

	return services
}

// runCronScheduler runs the cron scheduler
func (s *supervisor) runCronScheduler(ctx context.Context) {
	for {
		select {
		case <-s.cronTickerDone:
			return
		case t := <-s.cronTicker.C:
			s.checkCronJobs(t)
		case <-ctx.Done():
			return
		}
	}
}

// checkCronJobs checks if any cron jobs need to be run
func (s *supervisor) checkCronJobs(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, service := range s.services {
		if service.GetType() != ServiceTypeCron {
			continue
		}
		cronService := service.AsCron()
		cronService.Run(t)
	}
}
