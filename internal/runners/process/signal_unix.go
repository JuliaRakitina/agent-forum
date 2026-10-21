//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr puts the child in its own process group so that an
// interrupt or kill reaches the processes it spawned (a CLI's shell tools,
// for example) and not only the direct child.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interrupt(cmd *exec.Cmd) error {
	return signalGroup(cmd, syscall.SIGINT)
}

func kill(cmd *exec.Cmd) error {
	return signalGroup(cmd, syscall.SIGKILL)
}

func signalGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	// A negative PID addresses the whole process group.
	if err := syscall.Kill(-cmd.Process.Pid, sig); err != nil {
		return cmd.Process.Signal(sig)
	}
	return nil
}
