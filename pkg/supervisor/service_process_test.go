package supervisor

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func testSleepCmd() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "ping", "127.0.0.1", "-n", "3", ">", "NUL"}
	}
	return "sleep", []string{"1"}
}

func testEchoCmd(msg string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "echo", msg}
	}
	return "echo", []string{msg}
}

func TestProcessBasicLifecycle(t *testing.T) {
	path, args := testEchoCmd("hello")
	proc := NewProcess(path, args)

	stdout := make(chan string, 10)
	stderr := make(chan string, 10)
	exit := make(chan int, 1)

	proc.OnStdout(stdout)
	proc.OnStderr(stderr)
	proc.OnExit(exit)

	if proc.IsRunning() {
		t.Fatal("process should not be running before Start")
	}

	if err := proc.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	if !proc.IsRunning() {
		t.Fatal("process should be running after Start")
	}

	// Wait for process to exit
	select {
	case code := <-exit:
		if code != 0 {
			t.Errorf("unexpected exit code: %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for process exit")
	}

	if proc.IsRunning() {
		t.Error("process should not be running after exit")
	}

	// Check stdout
	found := false
	for line := range stdout {
		if strings.Contains(line, "hello") {
			found = true
		}
	}
	if !found {
		t.Error("did not receive expected stdout")
	}
}

func TestProcessDoubleStart(t *testing.T) {
	path, args := testSleepCmd()
	proc := NewProcess(path, args)
	if err := proc.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}
	err := proc.Start()
	if err == nil {
		t.Fatal("expected error on double Start")
	}
	_ = proc.Kill()
}

func TestProcessStopAndKill(t *testing.T) {
	path, args := testSleepCmd()
	proc := NewProcess(path, args)
	if err := proc.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}
	err := proc.Stop(100 * time.Millisecond)
	if err != nil && !strings.Contains(err.Error(), "unable to stop process") {
		t.Errorf("unexpected error on Stop: %v", err)
	}
	if proc.IsRunning() {
		_ = proc.Kill()
	}
}

func TestProcessKillNotRunning(t *testing.T) {
	path, args := testSleepCmd()
	proc := NewProcess(path, args)
	err := proc.Kill()
	if err == nil {
		t.Fatal("expected error on Kill when not running")
	}
}

func TestProcessRaceConditions(t *testing.T) {
	path, args := testSleepCmd()
	proc := NewProcess(path, args)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proc.IsRunning()
		}()
	}
	wg.Wait()
}

func TestProcessGoroutineCleanup(t *testing.T) {
	path, args := testSleepCmd()
	proc := NewProcess(path, args)
	if err := proc.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}
	_ = proc.Kill()
	// Wait a bit to allow goroutines to exit
	time.Sleep(100 * time.Millisecond)
	// No direct way to check goroutine leaks, but this should not deadlock or panic
}
