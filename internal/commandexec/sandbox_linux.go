//go:build linux

package commandexec

import (
	"manifold/internal/config"
)

func wrapSandbox(cfg config.ExecConfig, workdir, resolvedCommand string, args, env []string) (string, []string, bool, error) {
	if !boolDefault(cfg.Sandbox.Enabled, true) {
		return resolvedCommand, append([]string(nil), args...), false, nil
	}
	if boolDefault(cfg.Sandbox.Network.Enabled, false) && len(cfg.Sandbox.Network.AllowedDomains) > 0 {
		return "", nil, false, policyDeny("sandbox network domain allowlists are not supported by bwrap", "", false)
	}
	bwrap, err := lookPath("bwrap", pathFromEnv(env))
	if err != nil {
		if boolDefault(cfg.Sandbox.FailIfUnavailable, true) {
			return "", nil, false, policyDeny("bwrap is required but unavailable", "", false)
		}
		return resolvedCommand, append([]string(nil), args...), false, nil
	}
	wrappedArgs := []string{
		"--die-with-parent",
		"--unshare-all",
		"--new-session",
		"--proc", "/proc",
		"--dev", "/dev",
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/bin", "/bin",
		"--ro-bind-try", "/sbin", "/sbin",
		"--ro-bind-try", "/lib", "/lib",
		"--ro-bind-try", "/lib64", "/lib64",
		"--ro-bind-try", "/opt", "/opt",
		"--bind", workdir, workdir,
		"--chdir", workdir,
		"--setenv", "PATH", pathFromEnv(env),
		"--setenv", "HOME", workdir,
		"--setenv", "TMPDIR", workdir + "/.tmp",
		"--setenv", "TEMP", workdir + "/.tmp",
		"--setenv", "TMP", workdir + "/.tmp",
		"--setenv", "GOTOOLCHAIN", "local",
		"--setenv", "GO111MODULE", "on",
		"--setenv", "GOCACHE", workdir + "/.cache/go-build",
		"--setenv", "GOMODCACHE", workdir + "/.cache/go-mod",
		"--setenv", "TERM", "xterm-256color",
	}
	if boolDefault(cfg.Sandbox.Network.Enabled, false) {
		wrappedArgs = append(wrappedArgs, "--share-net")
	}
	wrappedArgs = append(wrappedArgs, "--", resolvedCommand)
	wrappedArgs = append(wrappedArgs, args...)
	return bwrap, wrappedArgs, true, nil
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
