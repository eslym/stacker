//go:build linux

package supervisor

import "syscall"

var sysProcAttr = syscall.SysProcAttr{
	Setpgid:   true,
	Pdeathsig: syscall.SIGKILL,
}
