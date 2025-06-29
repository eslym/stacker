//go:build windows

package config

import (
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
)

func getResolutionPath() []string {
	paths := getBaseResolutionPath()

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Stacker`, registry.READ)
	if err == nil {
		v, _, err := key.GetStringValue("ConfigPath")
		if err == nil && v != "" {
			paths = append(paths, v)
		}
		_ = key.Close()
	}

	if os.Getenv("ProgramData") != "" {
		paths = append(paths, filepath.Join(os.Getenv("ProgramData"), "Stacker"))
	}

	return paths
}
