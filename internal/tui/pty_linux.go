//go:build linux

package tui

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openPTY(width, height int) (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*os.File, *os.File, error) { master.Close(); return nil, nil, err }
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		return fail(err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		return fail(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return fail(err)
	}
	if err := resizePTY(master, width, height); err != nil {
		slave.Close()
		return fail(err)
	}
	return master, slave, nil
}
