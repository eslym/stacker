//go:build windows

package config

// GetDefaultConfigPath returns the default config path for Windows
func GetDefaultConfigPath() string {
	return "C:\\ProgramData\\Stacker\\config"
}