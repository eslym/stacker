package cli_test

import (
	"reflect"
	"testing"

	"github.com/eslym/stacker/pkg/cli"
)

func TestParseArgs_ConfigPath(t *testing.T) {
	// Test with --config flag
	args := []string{"--config", "/path/to/config.yaml"}
	options, err := cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	if options.ConfigPath != "/path/to/config.yaml" {
		t.Errorf("Expected ConfigPath to be /path/to/config.yaml, got %s", options.ConfigPath)
	}

	// Test with -c shorthand flag
	args = []string{"-c", "/path/to/config.yaml"}
	options, err = cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	if options.ConfigPath != "/path/to/config.yaml" {
		t.Errorf("Expected ConfigPath to be /path/to/config.yaml, got %s", options.ConfigPath)
	}
}

func TestParseArgs_With(t *testing.T) {
	// Test with --with flag
	args := []string{"--with", "service1,service2"}
	options, err := cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	expected := []string{"service1", "service2"}
	if !reflect.DeepEqual(options.With, expected) {
		t.Errorf("Expected With to be %v, got %v", expected, options.With)
	}

	// Test with -w shorthand flag
	args = []string{"-w", "service1,service2"}
	options, err = cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	if !reflect.DeepEqual(options.With, expected) {
		t.Errorf("Expected With to be %v, got %v", expected, options.With)
	}

	// Test with spaces in the list
	args = []string{"--with", "service1, service2"}
	options, err = cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	if !reflect.DeepEqual(options.With, expected) {
		t.Errorf("Expected With to be %v, got %v", expected, options.With)
	}
}

func TestParseArgs_Without(t *testing.T) {
	// Test with --without flag
	args := []string{"--without", "service1,service2"}
	options, err := cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	expected := []string{"service1", "service2"}
	if !reflect.DeepEqual(options.Without, expected) {
		t.Errorf("Expected Without to be %v, got %v", expected, options.Without)
	}

	// Test with -x shorthand flag
	args = []string{"-x", "service1,service2"}
	options, err = cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	if !reflect.DeepEqual(options.Without, expected) {
		t.Errorf("Expected Without to be %v, got %v", expected, options.Without)
	}
}

func TestParseArgs_Except(t *testing.T) {
	// Test with --except flag
	args := []string{"--except", "service1", "service2"}
	options, err := cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	if !options.Except {
		t.Errorf("Expected Except to be true, got false")
	}
	expected := []string{"service1", "service2"}
	if !reflect.DeepEqual(options.Services, expected) {
		t.Errorf("Expected Services to be %v, got %v", expected, options.Services)
	}

	// Test with -e shorthand flag
	args = []string{"-e", "service1", "service2"}
	options, err = cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	if !options.Except {
		t.Errorf("Expected Except to be true, got false")
	}
	if !reflect.DeepEqual(options.Services, expected) {
		t.Errorf("Expected Services to be %v, got %v", expected, options.Services)
	}
}

func TestParseArgs_Services(t *testing.T) {
	// Test with positional arguments
	args := []string{"service1", "service2"}
	options, err := cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	expected := []string{"service1", "service2"}
	if !reflect.DeepEqual(options.Services, expected) {
		t.Errorf("Expected Services to be %v, got %v", expected, options.Services)
	}

	// Test with flags and positional arguments
	args = []string{"--config", "/path/to/config.yaml", "service1", "service2"}
	options, err = cli.ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	if options.ConfigPath != "/path/to/config.yaml" {
		t.Errorf("Expected ConfigPath to be /path/to/config.yaml, got %s", options.ConfigPath)
	}
	if !reflect.DeepEqual(options.Services, expected) {
		t.Errorf("Expected Services to be %v, got %v", expected, options.Services)
	}
}

func TestValidateOptions_ExceptWithoutServices(t *testing.T) {
	// Test with --except flag but no services
	args := []string{"--except"}
	_, err := cli.ParseArgs(args)
	if err == nil {
		t.Errorf("Expected error for --except without services, got nil")
	}
}

func TestValidateOptions_WithAndWithout(t *testing.T) {
	// Test with both --with and --without flags
	args := []string{"--with", "service1", "--without", "service2"}
	_, err := cli.ParseArgs(args)
	if err != nil {
		t.Errorf("Expected no error for both --with and --without, got %v", err)
	}
}

func TestGetServiceSelection_SpecificServices(t *testing.T) {
	// Test with specific services
	options := &cli.Options{
		Services: []string{"service1", "service2"},
	}
	availableServices := map[string]bool{
		"service1": true,
		"service2": true,
		"service3": true,
	}
	optionalServices := map[string]bool{}
	result, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("Failed to get service selection: %v", err)
	}
	expected := map[string]bool{
		"service1": true,
		"service2": true,
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected result to be %v, got %v", expected, result)
	}
}

func TestGetServiceSelection_ExceptServices(t *testing.T) {
	// Test with --except flag
	options := &cli.Options{
		Except:   true,
		Services: []string{"service3"},
	}
	availableServices := map[string]bool{
		"service1": true,
		"service2": true,
		"service3": true,
	}
	optionalServices := map[string]bool{}
	result, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("Failed to get service selection: %v", err)
	}
	expected := map[string]bool{
		"service1": true,
		"service2": true,
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected result to be %v, got %v", expected, result)
	}
}

func TestGetServiceSelection_AllServices(t *testing.T) {
	// Test with no services specified (all services)
	options := &cli.Options{}
	availableServices := map[string]bool{
		"service1": false,
		"service2": false,
		"service3": true, // Optional service
	}
	optionalServices := map[string]bool{
		"service3": true,
	}
	result, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("Failed to get service selection: %v", err)
	}
	expected := map[string]bool{
		"service1": true,
		"service2": true,
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected result to be %v, got %v", expected, result)
	}
}

func TestGetServiceSelection_WithOptionalServices(t *testing.T) {
	// Test with --with flag for optional services
	options := &cli.Options{
		With: []string{"service3"},
	}
	availableServices := map[string]bool{
		"service1": false,
		"service2": false,
		"service3": true, // Optional service
	}
	optionalServices := map[string]bool{
		"service3": true,
	}
	result, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("Failed to get service selection: %v", err)
	}
	expected := map[string]bool{
		"service1": true,
		"service2": true,
		"service3": true,
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected result to be %v, got %v", expected, result)
	}
}

func TestGetServiceSelection_WithoutServices(t *testing.T) {
	// Test with --without flag
	options := &cli.Options{
		Without: []string{"service2"},
	}
	availableServices := map[string]bool{
		"service1": false,
		"service2": false,
		"service3": true, // Optional service
	}
	optionalServices := map[string]bool{
		"service3": true,
	}
	result, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("Failed to get service selection: %v", err)
	}
	expected := map[string]bool{
		"service1": true,
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected result to be %v, got %v", expected, result)
	}
}

func TestGetServiceSelection_UnknownService(t *testing.T) {
	// Test with unknown service
	options := &cli.Options{
		Services: []string{"unknown"},
	}
	availableServices := map[string]bool{
		"service1": true,
		"service2": true,
	}
	optionalServices := map[string]bool{}
	_, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err == nil {
		t.Errorf("Expected error for unknown service, got nil")
	}
}

func TestGetServiceSelection_UnknownOptionalService(t *testing.T) {
	// Test with unknown optional service
	options := &cli.Options{
		With: []string{"unknown"},
	}
	availableServices := map[string]bool{
		"service1": true,
		"service2": true,
	}
	optionalServices := map[string]bool{}
	_, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err == nil {
		t.Errorf("Expected error for unknown optional service, got nil")
	}
}

func TestGetServiceSelection_NonOptionalService(t *testing.T) {
	// Test with non-optional service in --with flag
	options := &cli.Options{
		With: []string{"service1"},
	}
	availableServices := map[string]bool{
		"service1": true,
		"service2": true,
	}
	optionalServices := map[string]bool{}
	result, err := cli.GetServiceSelection(options, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("Failed to get service selection: %v", err)
	}
	expected := map[string]bool{
		"service1": true,
		"service2": true,
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected result to be %v, got %v", expected, result)
	}
}
