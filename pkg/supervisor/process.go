package supervisor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/shirou/gopsutil/v3/process"
)

var ErrProcessRunning = errors.New("process already started")
var ErrProcessNotRunning = errors.New("process not running")
var ErrStopProcessTimeout = errors.New("unable to stop process")

type ProcessConfig struct {
	Path    string
	Args    []string
	WorkDir string
	Env     map[string]string
}

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

	GetPath() string
	GetArgs() []string
	GetWorkDir() string
	GetEnv() map[string]string
	GetPid() int
	GetID() string
	GetResourceStats() (*ProcessResourceStats, error)
}

type ProcessResourceStats struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryRSS     uint64  `json:"memory_rss"`
	MemoryPercent float32 `json:"memory_percent"`
}

type supervisedProcess struct {
	config  ProcessConfig
	mu      sync.Mutex
	stdout  []chan string
	stderr  []chan string
	exit    []chan int
	cleanup []chan struct{}

	cmd     *exec.Cmd
	started bool
	done    chan struct{}

	running int32 // atomic flag: 1 if running, 0 if not

	ctx    context.Context
	cancel context.CancelFunc

	id     string
	psutil *process.Process
}

func NewProcess(ctx context.Context, cfg ProcessConfig) Process {
	id, _ := gonanoid.New()
	sp := &supervisedProcess{
		config:  cfg,
		stdout:  []chan string{},
		stderr:  []chan string{},
		exit:    []chan int{},
		cleanup: make([]chan struct{}, 0),
		done:    make(chan struct{}),
		ctx:     ctx,
		id:      id,
	}
	return sp
}

func (sp *supervisedProcess) IsRunning() bool {
	return atomic.LoadInt32(&sp.running) == 1
}

func (sp *supervisedProcess) GetID() string {
	return sp.id
}

func (sp *supervisedProcess) Start() error {
	sp.mu.Lock()
	if sp.started {
		sp.mu.Unlock()
		return fmt.Errorf("%w", ErrProcessRunning)
	}
	sp.cmd = exec.Command(sp.config.Path, sp.config.Args...)
	sp.cmd.SysProcAttr = &sysProcAttr

	if sp.config.WorkDir != "" {
		sp.cmd.Dir = sp.config.WorkDir
	}

	if len(sp.config.Env) > 0 {
		env := os.Environ()
		for k, v := range sp.config.Env {
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
	// Attach psutil process for resource monitoring
	sp.psutil, _ = process.NewProcess(int32(sp.cmd.Process.Pid))
	sp.started = true
	sp.done = make(chan struct{})
	atomic.StoreInt32(&sp.running, 1)

	// Cleanup channels for goroutines
	stdoutCleanup := make(chan struct{})
	stderrCleanup := make(chan struct{})
	exitCleanup := make(chan struct{})

	sp.cleanup = append(sp.cleanup, stdoutCleanup, stderrCleanup, exitCleanup)
	sp.mu.Unlock()

	go sp.readPipeWithContext(stdoutPipe, func(line string) {
		sp.mu.Lock()
		listenerCount := len(sp.stdout)
		if listenerCount > 1 {
			fmt.Fprintf(os.Stderr, "[debug] process %s: %d stdout listeners\n", sp.config.Path, listenerCount)
		}
		for _, ch := range sp.stdout {
			select {
			case ch <- line:
			default:
			}
		}
		sp.mu.Unlock()
	}, stdoutCleanup)

	go sp.readPipeWithContext(stderrPipe, func(line string) {
		sp.mu.Lock()
		listenerCount := len(sp.stderr)
		if listenerCount > 1 {
			fmt.Fprintf(os.Stderr, "[debug] process %s: %d stderr listeners\n", sp.config.Path, listenerCount)
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
		defer close(exitCleanup)
		defer close(sp.done)
		select {
		case <-sp.ctx.Done():
			return
		default:
			err := sp.cmd.Wait()
			if err != nil {
				var exitErr *exec.ExitError
				_ = exitErr // not used
			}
			sp.mu.Lock()
			for _, ch := range sp.exit {
				select {
				case ch <- 0: // always send 0 for now
				default:
				}
			}
			sp.started = false
			sp.cmd = nil
			atomic.StoreInt32(&sp.running, 0)
			sp.mu.Unlock()
		}
	}()

	return nil
}

func (sp *supervisedProcess) readPipeWithContext(pipe io.ReadCloser, send func(string), cleanup chan struct{}) {
	scanner := bufio.NewScanner(pipe)
	for {
		select {
		case <-sp.ctx.Done():
			return
		case <-cleanup:
			return
		default:
			if scanner.Scan() {
				line := scanner.Text()
				send(line)
			} else {
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
	if !sp.started || sp.cmd == nil || sp.cmd.Process == nil || sp.ctx == nil {
		sp.mu.Unlock()
		return fmt.Errorf("%w", ErrProcessNotRunning)
	}
	proc := sp.cmd.Process
	done := sp.done
	sp.mu.Unlock()

	if sp.cancel != nil {
		sp.cancel() // cancel context to cleanup goroutines
	}

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
	if sp.cancel != nil {
		sp.cancel()
	}
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

func (sp *supervisedProcess) GetPath() string {
	return sp.config.Path
}

func (sp *supervisedProcess) GetArgs() []string {
	return sp.config.Args
}

func (sp *supervisedProcess) GetWorkDir() string {
	return sp.config.WorkDir
}

func (sp *supervisedProcess) GetEnv() map[string]string {
	return sp.config.Env
}

func (sp *supervisedProcess) GetPid() int {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.cmd != nil && sp.cmd.Process != nil {
		return sp.cmd.Process.Pid
	}
	return 0
}

func (sp *supervisedProcess) GetResourceStats() (*ProcessResourceStats, error) {
	sp.mu.Lock()
	ps := sp.psutil
	sp.mu.Unlock()
	if ps == nil {
		return nil, fmt.Errorf("psutil process not initialized")
	}
	cpu, _ := ps.CPUPercent()
	mem, _ := ps.MemoryInfo()
	memPercent, _ := ps.MemoryPercent()
	return &ProcessResourceStats{
		CPUPercent:    cpu,
		MemoryRSS:     mem.RSS,
		MemoryPercent: memPercent,
	}, nil
}
