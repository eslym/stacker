package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// Options represents the command-line options
type Options struct {
	ConfigPath string
	With       []string
	Without    []string
	Except     bool
	Services   []string
	Verbose    bool
}

// ParseArgs parses the command-line arguments
func ParseArgs(args []string) (*Options, error) {
	options := &Options{}

	// Create a custom flag set
	fs := flag.NewFlagSet("stacker", flag.ContinueOnError)

 // Define flags
	fs.StringVar(&options.ConfigPath, "config", "", "Path to config file (JSON or YAML)")
	fs.StringVar(&options.ConfigPath, "c", "", "Path to config file (JSON or YAML) (shorthand)")

	// Define slice flags
	var with, without string
	fs.StringVar(&with, "with", "", "Comma-separated list of services to include")
	fs.StringVar(&with, "w", "", "Comma-separated list of services to include (shorthand)")
	fs.StringVar(&without, "without", "", "Comma-separated list of services to exclude")
	fs.StringVar(&without, "x", "", "Comma-separated list of services to exclude (shorthand)")
	fs.BoolVar(&options.Except, "except", false, "Exclude specified services from all non-optional services")
	fs.BoolVar(&options.Except, "e", false, "Exclude specified services from all non-optional services (shorthand)")

	// Add help flag
	var help bool
	fs.BoolVar(&help, "help", false, "Show help message")
	fs.BoolVar(&help, "h", false, "Show help message (shorthand)")

	// Add verbose flag
	fs.BoolVar(&options.Verbose, "v", false, "Enable verbose output")

	// Parse flags
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Check if help flag is provided
	if help {
		fs.SetOutput(os.Stdout)
		fmt.Println("Stacker - Service Supervisor")
		fmt.Println("\nUsage:")
		fmt.Println("  stacker [options] [services...]")
		fmt.Println("\nOptions:")
		fs.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  # Run all non-optional services")
		fmt.Println("  stacker")
		fmt.Println("\n  # Run only specified services")
		fmt.Println("  stacker service1 service2")
		fmt.Println("\n  # Run all non-optional services except the specified ones")
		fmt.Println("  stacker --except service1 service2")
		fmt.Println("\n  # Run all non-optional services, exclude some, and include others")
		fmt.Println("  stacker --without service1 --with service2")
		fmt.Println("\n  # Run with verbose output")
		fmt.Println("  stacker -v")
		os.Exit(0)
	}

	// Process slice flags
	if with != "" {
		options.With = strings.Split(with, ",")
		for i, s := range options.With {
			options.With[i] = strings.TrimSpace(s)
		}
	}

	if without != "" {
		options.Without = strings.Split(without, ",")
		for i, s := range options.Without {
			options.Without[i] = strings.TrimSpace(s)
		}
	}

	// Get positional arguments (services)
	options.Services = fs.Args()

	// Validate options
	if err := validateOptions(options); err != nil {
		return nil, err
	}

	return options, nil
}

// validateOptions validates the command-line options
func validateOptions(options *Options) error {
	// Check for conflicting options
	if options.Except && len(options.Services) == 0 {
		return errors.New("--except flag requires service names")
	}

	return nil
}

// GetServiceSelection returns the list of services to run based on the options
func GetServiceSelection(options *Options, availableServices map[string]bool, optionalServices map[string]bool) (map[string]bool, error) {
	result := make(map[string]bool)

	// Case 1: Run specific services
	if len(options.Services) > 0 {
		if options.Except {
			// Run all non-optional services except the specified ones
			for service := range availableServices {
				if !optionalServices[service] {
					result[service] = true
				}
			}
			for _, service := range options.Services {
				if _, exists := availableServices[service]; !exists {
					return nil, fmt.Errorf("unknown service: %s", service)
				}
				delete(result, service)
			}
		} else {
			// Run only the specified services
			for _, service := range options.Services {
				if _, exists := availableServices[service]; !exists {
					return nil, fmt.Errorf("unknown service: %s", service)
				}
				result[service] = true
			}
		}
	} else {
		// Run all services (except optional ones by default)
		for service := range availableServices {
			if !optionalServices[service] {
				result[service] = true
			}
		}
	}

	// Apply --with flag (explicitly include services)
	for _, service := range options.With {
		if _, exists := availableServices[service]; !exists {
			return nil, fmt.Errorf("unknown service: %s", service)
		}
		result[service] = true
	}

	// Apply --without flag (explicitly exclude services)
	for _, service := range options.Without {
		if _, exists := availableServices[service]; !exists {
			return nil, fmt.Errorf("unknown service: %s", service)
		}
		delete(result, service)
	}

	return result, nil
}
