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

// alive reports whether the direct child has not been reaped yet. Once
// cmd.Wait has collected the exit status, signal 0 fails with ESRCH.
func alive(cmd *exec.Cmd) bool {
	if cmd.Process == nil {
		return false
	}
	return syscall.Kill(cmd.Process.Pid, 0) == nil
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
