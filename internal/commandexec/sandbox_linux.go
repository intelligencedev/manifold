//go:build linux

package commandexec

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"manifold/internal/config"
	"manifold/internal/egress"
)

func wrapSandbox(cfg config.ExecConfig, workdir, resolvedCommand string, args, env []string, network *sandboxNetwork) (string, []string, bool, error) {
	if !boolDefault(cfg.Sandbox.Enabled, true) {
		return resolvedCommand, append([]string(nil), args...), false, nil
	}
	bwrap, err := lookPath("bwrap", pathFromEnv(env))
	if err != nil {
		if network != nil && network.mode == networkModeDomainLimited {
			return "", nil, false, policyDeny("bwrap is required for domain-limited network egress", "", false)
		}
		return "", nil, false, policyDeny("bwrap is required but unavailable", "", false)
	}
	command := resolvedCommand
	commandArgs := append([]string(nil), args...)
	if network != nil && network.mode == networkModeDomainLimited {
		command, commandArgs, err = prepareLinuxSupervisor(workdir, resolvedCommand, args, env, network)
		if err != nil {
			return "", nil, false, err
		}
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
		if network == nil || network.mode != networkModeDomainLimited {
			wrappedArgs = append(wrappedArgs, "--share-net")
		}
	}
	wrappedArgs = appendProxySetenv(wrappedArgs, env)
	wrappedArgs = append(wrappedArgs, "--", command)
	wrappedArgs = append(wrappedArgs, commandArgs...)
	return bwrap, wrappedArgs, true, nil
}

func prepareLinuxSupervisor(workdir, resolvedCommand string, args, env []string, network *sandboxNetwork) (string, []string, error) {
	helperPath, err := copySupervisorHelper(workdir)
	if err != nil {
		return "", nil, policyDeny("prepare egress supervisor: "+err.Error(), "", false)
	}
	spec := egress.SupervisorSpec{
		Command:          resolvedCommand,
		Args:             append([]string(nil), args...),
		Env:              append([]string(nil), env...),
		Dir:              workdir,
		BrokerSocketPath: network.brokerSocket,
		ListenAddress:    network.listenAddress,
	}
	specPath, err := writeSupervisorSpec(workdir, spec)
	if err != nil {
		_ = os.Remove(helperPath)
		return "", nil, policyDeny("write egress supervisor spec: "+err.Error(), "", false)
	}
	network.cleanupFiles = append(network.cleanupFiles, helperPath, specPath)
	previousCleanup := network.cleanup
	network.cleanup = func() {
		runCleanup(previousCleanup)
		for _, path := range network.cleanupFiles {
			_ = os.Remove(path)
		}
	}
	return helperPath, []string{egress.SupervisorFlag, specPath}, nil
}

func copySupervisorHelper(workdir string) (string, error) {
	source, err := os.Executable()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(workdir, ".tmp", "manifold-egress-supervisor")
	src, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer src.Close()
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o700); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func writeSupervisorSpec(workdir string, spec egress.SupervisorSpec) (string, error) {
	file, err := os.CreateTemp(filepath.Join(workdir, ".tmp"), "egress-supervisor-*.json")
	if err != nil {
		return "", err
	}
	defer file.Close()
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	if _, err := file.Write(encoded); err != nil {
		return "", err
	}
	return file.Name(), nil
}

func appendProxySetenv(args, env []string) []string {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy", "NO_PROXY", "no_proxy", "GIT_HTTP_PROXY", "GIT_HTTPS_PROXY", "GIT_SSH_COMMAND"} {
		if value := envValue(env, key); value != "" {
			args = append(args, "--setenv", key, value)
		}
	}
	return args
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
