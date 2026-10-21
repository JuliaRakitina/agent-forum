//go:build windows

package process

import "os/exec"

// Windows has no SIGINT for non-console children; the interrupt step is a
// no-op and the grace period ends in a kill. Killing the process tree needs
// a job object and is left for the Windows port.
func setSysProcAttr(cmd *exec.Cmd) {}

func alive(cmd *exec.Cmd) bool { return cmd.Process != nil }

func interrupt(cmd *exec.Cmd) error { return nil }

func kill(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
