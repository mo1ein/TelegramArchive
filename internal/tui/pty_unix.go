//go:build darwin || linux

package tui

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func configurePTY(child *exec.Cmd) {
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
}
func resizePTY(file *os.File, width, height int) error {
	return unix.IoctlSetWinsize(int(file.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(max(20, min(width, 500))), Row: uint16(max(5, min(height, 200)))})
}
func interruptJob(child *exec.Cmd) {
	if child.SysProcAttr != nil && child.SysProcAttr.Setsid {
		_ = unix.Kill(-child.Process.Pid, unix.SIGINT)
	} else {
		_ = child.Process.Signal(os.Interrupt)
	}
}
func killJob(child *exec.Cmd) {
	if child.SysProcAttr != nil && child.SysProcAttr.Setsid {
		_ = unix.Kill(-child.Process.Pid, unix.SIGKILL)
	} else {
		_ = child.Process.Kill()
	}
}
