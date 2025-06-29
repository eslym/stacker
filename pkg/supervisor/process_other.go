//go:build !windows && !linux

package supervisor

import "syscall"

var sysProcAttr = syscall.SysProcAttr{
	Setpgid: true,
}
