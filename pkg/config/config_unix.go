//go:build !windows && !darwin

package config

// GetDefaultConfigPath returns the default config path for Linux and other Unix-like systems
func GetDefaultConfigPath() string {
	return "/usr/local/etc/stacker/config"
}