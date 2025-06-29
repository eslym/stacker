//go:build windows

package supervisor

import (
	"syscall"
)

var sysProcAttr = syscall.SysProcAttr{
	CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
}
