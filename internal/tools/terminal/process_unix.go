//go:build !windows

package terminal

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
)

func startProcessPTY(cmd *exec.Cmd, rows, cols int) (*os.File, error) {
	if rows <= 0 {
		rows = 24
	}
	if cols <= 0 {
		cols = 80
	}
	return pty.StartWithAttrs(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}, &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	})
}

func terminateProcess(cmd *exec.Cmd, signalName string, grace time.Duration, done <-chan struct{}) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	sig := parseSignal(signalName)
	pid := cmd.Process.Pid
	if err := syscall.Kill(-pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	if sig == syscall.SIGKILL || grace <= 0 {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(grace):
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func parseSignal(signalName string) syscall.Signal {
	switch strings.ToLower(strings.TrimSpace(signalName)) {
	case "kill", "sigkill", "9":
		return syscall.SIGKILL
	case "term", "terminate", "sigterm", "15":
		return syscall.SIGTERM
	default:
		return syscall.SIGINT
	}
}
