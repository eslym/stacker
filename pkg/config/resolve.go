package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func ResolveConfigPath(files ...string) (string, error) {
	if value, exists := os.LookupEnv("STACKER_CONFIG_PATH"); exists {
		return value, nil
	}

	lookup := getResolutionPath()

	for _, lookup := range lookup {
		for _, file := range files {
			path := filepath.Join(lookup, file)
			stat, err := os.Stat(path)
			if err == nil && !stat.IsDir() {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("config file not found in %v", lookup)
}

func getBaseResolutionPath() []string {
	var paths []string

	if dir, err := os.Getwd(); err == nil {
		paths = append(paths, dir)
	}

	if exe, err := os.Executable(); err == nil {
		path := filepath.Dir(filepath.Dir(exe))
		paths = append(paths, path)
	}

	if dir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(dir, "Stacker"))
	}

	return paths
}
