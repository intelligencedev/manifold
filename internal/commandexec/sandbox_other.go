//go:build !darwin && !linux

package commandexec

import (
	"runtime"

	"manifold/internal/config"
)

func wrapSandbox(cfg config.ExecConfig, workdir, resolvedCommand string, args, env []string, network *sandboxNetwork) (string, []string, bool, error) {
	if !boolDefault(cfg.Sandbox.Enabled, true) {
		return resolvedCommand, append([]string(nil), args...), false, nil
	}
	return "", nil, false, policyDeny("command sandbox is not supported on "+runtime.GOOS, "", false)
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
