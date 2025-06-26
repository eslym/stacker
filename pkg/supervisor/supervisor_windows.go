//go:build windows

package supervisor

import (
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// prepareCommand configures a command for Windows-specific behavior
func prepareCommand(cmd *exec.Cmd) {
	// Put the child in its own console process group
	// so that we can address it without killing ourselves
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags = syscall.CREATE_NEW_PROCESS_GROUP
}

// stopProcess attempts to gracefully stop a process on Windows
// If graceful stop fails or times out, it will force kill the process
func stopProcess(process *os.Process, timeout time.Duration) error {
	// Try to send CTRL_BREAK_EVENT directly to the process
	err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(process.Pid))

	// If direct method fails, try using os.Interrupt (maps to CTRL_BREAK_EVENT on Windows in Go 1.22+)
	if err != nil {
		err = process.Signal(os.Interrupt)
	}

	// If both methods fail, try force kill
	if err != nil {
		return forceKill(process)
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
		return forceKill(process)
	case <-done:
		// Process exited gracefully
		return nil
	}
}

// forceKill forcefully terminates a Windows process
// using the proper handle permissions
func forceKill(process *os.Process) error {
	// First try with just PROCESS_TERMINATE permission
	h, err := windows.OpenProcess(
		windows.PROCESS_TERMINATE,
		false, // do not inherit
		uint32(process.Pid),
	)

	// If that fails, try with PROCESS_TERMINATE|PROCESS_QUERY_INFORMATION
	if err != nil {
		h, err = windows.OpenProcess(
			windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_INFORMATION,
			false,
			uint32(process.Pid),
		)
	}

	// If that still fails, try with just the original permissions
	if err != nil {
		h, err = windows.OpenProcess(
			windows.PROCESS_TERMINATE|windows.SYNCHRONIZE,
			false,
			uint32(process.Pid),
		)
	}

	// If all OpenProcess attempts fail, fall back to Kill()
	if err != nil {
		return process.Kill()
	}

	defer windows.CloseHandle(h)

	// Terminate the process
	err = windows.TerminateProcess(h, 1)
	if err != nil {
		// If TerminateProcess fails, fall back to Kill()
		return process.Kill()
	}

	return nil
}
