//go:build !windows

package supervisor

import (
	"os"
	"os/exec"
	"time"
)

// prepareCommand is a no-op on Unix platforms
func prepareCommand(cmd *exec.Cmd) {
	// No special preparation needed for Unix platforms
}

// stopProcess attempts to gracefully stop a process on Unix platforms
// If graceful stop fails or times out, it will force kill the process
func stopProcess(process *os.Process, timeout time.Duration) error {
	// Send os.Interrupt (SIGINT on Unix) for graceful shutdown
	if err := process.Signal(os.Interrupt); err != nil {
		// If SIGINT fails, try force kill
		return process.Kill()
	}

	// Wait for the process to exit or timeout
	done := make(chan struct{})
	go func() {
		// Check if the process has exited
		for {
			if _, err := process.Wait(); err != nil {
				// Process has exited or error occurred
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		close(done)
	}()

	select {
	case <-time.After(timeout):
		// Timeout, process didn't exit gracefully
		return process.Kill()
	case <-done:
		// Process exited gracefully
		return nil
	}
}
