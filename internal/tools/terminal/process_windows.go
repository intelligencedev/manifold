//go:build windows

package terminal

import (
	"errors"
	"os"
	"os/exec"
	"time"
)

func startProcessPTY(cmd *exec.Cmd, rows, cols int) (*os.File, error) {
	return nil, errors.New("PTY-backed terminal sessions are not supported on this platform")
}

func terminateProcess(cmd *exec.Cmd, signalName string, grace time.Duration, done <-chan struct{}) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
