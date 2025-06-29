package supervisor

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

var ErrProcessRunning = errors.New("process already started")
var ErrProcessNotRunning = errors.New("process not running")
var ErrStopProcessTimeout = errors.New("unable to stop process")

type Process interface {
	Start() error
	Stop(timeout time.Duration) error
	Kill() error

	IsRunning() bool

	OnStdout(listener chan string)
	OnStderr(listener chan string)
	OnExit(listener chan int)

	OffStdout(listener chan string)
	OffStderr(listener chan string)
	OffExit(listener chan int)
}

type supervisedProcess struct {
	Path string
	Args []string

	WorkDir string
	Env     map[string]string

	mu sync.Mutex

	stdout  []chan string
	stderr  []chan string
	exit    []chan int
	cleanup []chan struct{}

	cmd     *exec.Cmd
	started bool
	done    chan struct{}

	running int32 // atomic flag: 1 if running, 0 if not
}

func NewProcess(path string, args []string) Process {
	sp := &supervisedProcess{
		Path:    path,
		Args:    args,
		stdout:  []chan string{},
		stderr:  []chan string{},
		exit:    []chan int{},
		cleanup: make([]chan struct{}, 0),
		done:    make(chan struct{}),
	}
	return sp
}

func (sp *supervisedProcess) IsRunning() bool {
	return atomic.LoadInt32(&sp.running) == 1
}

func (sp *supervisedProcess) Start() error {
	sp.mu.Lock()
	if sp.started {
		sp.mu.Unlock()
		return fmt.Errorf("%w", ErrProcessRunning)
	}
	sp.cmd = exec.Command(sp.Path, sp.Args...)
	sp.cmd.SysProcAttr = &sysProcAttr

	if sp.WorkDir != "" {
		sp.cmd.Dir = sp.WorkDir
	}

	if len(sp.Env) > 0 {
		env := os.Environ()
		for k, v := range sp.Env {
			env = append(env, k+"="+v)
		}
		sp.cmd.Env = env
	} else {
		sp.cmd.Env = os.Environ()
	}

	stdoutPipe, err := sp.cmd.StdoutPipe()
	if err != nil {
		sp.mu.Unlock()
		return err
	}
	stderrPipe, err := sp.cmd.StderrPipe()
	if err != nil {
		sp.mu.Unlock()
		return err
	}
	if err := sp.cmd.Start(); err != nil {
		sp.mu.Unlock()
		return err
	}
	sp.started = true
	sp.done = make(chan struct{})
	atomic.StoreInt32(&sp.running, 1)

	// Cleanup channels for goroutines
	stdoutCleanup := make(chan struct{})
	stderrCleanup := make(chan struct{})
	exitCleanup := make(chan struct{})

	sp.cleanup = append(sp.cleanup, stdoutCleanup, stderrCleanup, exitCleanup)
	sp.mu.Unlock()

	go sp.readPipe(stdoutPipe, func(line string) {
		sp.mu.Lock()
		listenerCount := len(sp.stdout)
		if listenerCount > 1 {
			fmt.Fprintf(os.Stderr, "[debug] process %s: %d stdout listeners\n", sp.Path, listenerCount)
		}
		for _, ch := range sp.stdout {
			select {
			case ch <- line:
			default:
			}
		}
		sp.mu.Unlock()
	}, stdoutCleanup)

	go sp.readPipe(stderrPipe, func(line string) {
		sp.mu.Lock()
		listenerCount := len(sp.stderr)
		if listenerCount > 1 {
			fmt.Fprintf(os.Stderr, "[debug] process %s: %d stderr listeners\n", sp.Path, listenerCount)
		}
		for _, ch := range sp.stderr {
			select {
			case ch <- line:
			default:
			}
		}
		sp.mu.Unlock()
	}, stderrCleanup)

	go func() {
		err := sp.cmd.Wait()
		exitCode := 0
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		sp.mu.Lock()
		for _, ch := range sp.exit {
			select {
			case ch <- exitCode:
			default:
			}
		}
		sp.started = false
		sp.cmd = nil
		atomic.StoreInt32(&sp.running, 0)
		sp.mu.Unlock()
		close(exitCleanup)
		close(sp.done)
	}()

	return nil
}

func (sp *supervisedProcess) readPipe(pipe io.ReadCloser, send func(string), cleanup chan struct{}) {
	scanner := bufio.NewScanner(pipe)
	for {
		select {
		case <-cleanup:
			return
		default:
			if scanner.Scan() {
				line := scanner.Text()
				send(line)
			} else {
				// Only flush if there is a partial line not already sent
				// But bufio.Scanner guarantees all complete lines are sent, so nothing to do here
				return
			}
		}
	}
}

func (sp *supervisedProcess) Stop(timeout time.Duration) error {
	sp.mu.Lock()
	if !sp.started || sp.cmd == nil || sp.cmd.Process == nil {
		sp.mu.Unlock()
		return fmt.Errorf("%w", ErrProcessNotRunning)
	}
	proc := sp.cmd.Process
	done := sp.done
	sp.mu.Unlock()

	// Try to send interrupt, fallback to kill if not supported
	err := proc.Signal(os.Interrupt)
	if err != nil {
		return err
	}

	select {
	case <-done:
		sp.postCleanup()
		return nil
	case <-time.After(timeout):
		sp.postCleanup()
		return fmt.Errorf("%w within %s", ErrStopProcessTimeout, timeout)
	}
}

func (sp *supervisedProcess) Kill() error {
	sp.mu.Lock()
	if !sp.started || sp.cmd == nil || sp.cmd.Process == nil {
		sp.mu.Unlock()
		return fmt.Errorf("%w", ErrProcessNotRunning)
	}
	proc := sp.cmd.Process
	done := sp.done
	sp.mu.Unlock()

	err := proc.Kill()
	<-done
	sp.postCleanup()
	return err
}

func (sp *supervisedProcess) postCleanup() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for _, ch := range sp.cleanup {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
	sp.cleanup = nil
	sp.started = false
	sp.cmd = nil
	atomic.StoreInt32(&sp.running, 0)
}

// ForceCleanup forcibly resets the process state (for test/restart recovery)
func (sp *supervisedProcess) ForceCleanup() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.cleanup = nil
	sp.started = false
	sp.cmd = nil
	atomic.StoreInt32(&sp.running, 0)
}

func (sp *supervisedProcess) OnStdout(listener chan string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for _, l := range sp.stdout {
		if l == listener {
			return
		}
	}
	sp.stdout = append(sp.stdout, listener)
}

func (sp *supervisedProcess) OnStderr(listener chan string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for _, l := range sp.stderr {
		if l == listener {
			return
		}
	}
	sp.stderr = append(sp.stderr, listener)
}

func (sp *supervisedProcess) OnExit(listener chan int) {
	sp.mu.Lock()
	sp.exit = append(sp.exit, listener)
	sp.mu.Unlock()
}

func (sp *supervisedProcess) OffStdout(listener chan string) {
	sp.mu.Lock()
	for i, ch := range sp.stdout {
		if ch == listener {
			sp.stdout = append(sp.stdout[:i], sp.stdout[i+1:]...)
			break
		}
	}
	sp.mu.Unlock()
}

func (sp *supervisedProcess) OffStderr(listener chan string) {
	sp.mu.Lock()
	for i, ch := range sp.stderr {
		if ch == listener {
			sp.stderr = append(sp.stderr[:i], sp.stderr[i+1:]...)
			break
		}
	}
	sp.mu.Unlock()
}

func (sp *supervisedProcess) OffExit(listener chan int) {
	sp.mu.Lock()
	for i, ch := range sp.exit {
		if ch == listener {
			sp.exit = append(sp.exit[:i], sp.exit[i+1:]...)
			break
		}
	}
	sp.mu.Unlock()
}
