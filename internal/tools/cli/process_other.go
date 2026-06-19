//go:build !unix

package cli

import "os/exec"

func configureCommandProcess(cmd *exec.Cmd) {}

func terminateCommandProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
