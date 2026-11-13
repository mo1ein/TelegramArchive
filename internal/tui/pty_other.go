//go:build !darwin && !linux

package tui

import (
	"fmt"
	"os"
	"os/exec"
)

func openPTY(width, height int) (*os.File, *os.File, error) {
	return nil, nil, fmt.Errorf("interactive embedded tasks currently require macOS or Linux")
}
func configurePTY(child *exec.Cmd)                     {}
func resizePTY(file *os.File, width, height int) error { return nil }
func interruptJob(child *exec.Cmd)                     { _ = child.Process.Kill() }
func killJob(child *exec.Cmd)                          { _ = child.Process.Kill() }
