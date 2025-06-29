//go:build !windows && !darwin

package config

func getResolutionPath() []string {
	paths := getBaseResolutionPath()

	paths = append(paths, "/etc/stacker")

	return paths
}
