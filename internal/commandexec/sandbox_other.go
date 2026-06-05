//go:build !darwin && !linux

package commandexec

import (
	"runtime"

	"manifold/internal/config"
)

func wrapSandbox(cfg config.ExecConfig, workdir, resolvedCommand string, args, env []string) (string, []string, bool, error) {
	if !boolDefault(cfg.Sandbox.Enabled, true) {
		return resolvedCommand, append([]string(nil), args...), false, nil
	}
	if boolDefault(cfg.Sandbox.FailIfUnavailable, true) {
		return "", nil, false, policyDeny("command sandbox is not supported on "+runtime.GOOS, "", false)
	}
	return resolvedCommand, append([]string(nil), args...), false, nil
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
