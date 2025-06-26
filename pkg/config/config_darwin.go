//go:build darwin

package config

// GetDefaultConfigPath returns the default config path for macOS
func GetDefaultConfigPath() string {
	return "/Library/Application Support/Stacker/config"
}