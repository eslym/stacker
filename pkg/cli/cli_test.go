package cli

import (
	"reflect"
	"testing"
)

var (
	availableServices = map[string]bool{
		"nginx":     true,
		"php-fpm":   true,
		"laravel":   true,
		"scheduler": true,
		"queue":     true,
		"horizon":   true,
	}
	optionalServices = map[string]bool{
		"queue":   true,
		"horizon": true,
	}
)

func TestParseArgs_Basic(t *testing.T) {
	args := []string{"--config", "my.yaml", "--with=queue,horizon", "--without=scheduler", "nginx", "php-fpm"}
	opts, err := ParseArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.ConfigPath != "my.yaml" {
		t.Errorf("ConfigPath: got %q, want %q", opts.ConfigPath, "my.yaml")
	}
	if !reflect.DeepEqual(opts.With, []string{"queue", "horizon"}) {
		t.Errorf("With: got %v, want %v", opts.With, []string{"queue", "horizon"})
	}
	if !reflect.DeepEqual(opts.Without, []string{"scheduler"}) {
		t.Errorf("Without: got %v, want %v", opts.Without, []string{"scheduler"})
	}
	if !reflect.DeepEqual(opts.Services, []string{"nginx", "php-fpm"}) {
		t.Errorf("Services: got %v, want %v", opts.Services, []string{"nginx", "php-fpm"})
	}
}

func TestParseArgs_Except(t *testing.T) {
	args := []string{"--except", "nginx", "php-fpm"}
	opts, err := ParseArgs(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.Except {
		t.Errorf("Except: got %v, want true", opts.Except)
	}
	if !reflect.DeepEqual(opts.Services, []string{"nginx", "php-fpm"}) {
		t.Errorf("Services: got %v, want %v", opts.Services, []string{"nginx", "php-fpm"})
	}
}

func TestParseArgs_ExceptRequiresService(t *testing.T) {
	args := []string{"--except"}
	_, err := ParseArgs(args)
	if err == nil {
		t.Fatal("expected error for --except with no services")
	}
}

func TestGetServiceSelection_Specific(t *testing.T) {
	opts := &Options{Services: []string{"nginx", "php-fpm"}}
	got, err := GetServiceSelection(opts, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]bool{"nginx": true, "php-fpm": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGetServiceSelection_Except(t *testing.T) {
	opts := &Options{Services: []string{"nginx", "php-fpm"}, Except: true}
	got, err := GetServiceSelection(opts, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// All non-optional except nginx, php-fpm
	want := map[string]bool{"laravel": true, "scheduler": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGetServiceSelection_AllNonOptional(t *testing.T) {
	opts := &Options{}
	got, err := GetServiceSelection(opts, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]bool{"nginx": true, "php-fpm": true, "laravel": true, "scheduler": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGetServiceSelection_WithAndWithout(t *testing.T) {
	opts := &Options{
		With:    []string{"queue"},
		Without: []string{"scheduler"},
	}
	got, err := GetServiceSelection(opts, availableServices, optionalServices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]bool{"nginx": true, "php-fpm": true, "laravel": true, "queue": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestGetServiceSelection_UnknownService(t *testing.T) {
	opts := &Options{Services: []string{"notfound"}}
	_, err := GetServiceSelection(opts, availableServices, optionalServices)
	if err == nil {
		t.Fatal("expected error for unknown service")
	}
}

func TestGetServiceSelection_WithUnknown(t *testing.T) {
	opts := &Options{With: []string{"notfound"}}
	_, err := GetServiceSelection(opts, availableServices, optionalServices)
	if err == nil {
		t.Fatal("expected error for unknown service in --with")
	}
}

func TestGetServiceSelection_WithoutUnknown(t *testing.T) {
	opts := &Options{Without: []string{"notfound"}}
	_, err := GetServiceSelection(opts, availableServices, optionalServices)
	if err == nil {
		t.Fatal("expected error for unknown service in --without")
	}
}
