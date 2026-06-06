//go:build darwin

package commandexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"manifold/internal/config"
)

func wrapSandbox(cfg config.ExecConfig, workdir, resolvedCommand string, args, env []string, network *sandboxNetwork) (string, []string, bool, error) {
	if !boolDefault(cfg.Sandbox.Enabled, true) {
		return resolvedCommand, append([]string(nil), args...), false, nil
	}
	sandboxExec := "/usr/bin/sandbox-exec"
	if _, err := os.Stat(sandboxExec); err != nil {
		if network != nil && network.mode == networkModeDomainLimited {
			return "", nil, false, policyDeny("sandbox-exec is required for domain-limited network egress", "", false)
		}
		if boolDefault(cfg.Sandbox.FailIfUnavailable, true) {
			return "", nil, false, policyDeny("sandbox-exec is required but unavailable", "", false)
		}
		return resolvedCommand, append([]string(nil), args...), false, nil
	}
	profile, err := macOSSandboxProfile(cfg, workdir, resolvedCommand, network)
	if err != nil {
		return "", nil, false, err
	}
	wrappedArgs := []string{"-p", profile, "--", resolvedCommand}
	wrappedArgs = append(wrappedArgs, args...)
	return sandboxExec, wrappedArgs, true, nil
}

func macOSSandboxProfile(cfg config.ExecConfig, workdir, resolvedCommand string, network *sandboxNetwork) (string, error) {
	writePaths := []string{workdir}
	if realWorkdir, err := filepath.EvalSymlinks(workdir); err == nil {
		writePaths = append(writePaths, realWorkdir)
	}

	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(allow default)\n")
	b.WriteString("(deny file-write*)\n")
	for _, path := range compactPaths(writePaths) {
		fmt.Fprintf(&b, "(allow file-write* (subpath %q))\n", path)
	}
	b.WriteString("(allow file-write* (literal \"/dev/null\"))\n")
	if network != nil && network.mode == networkModeDomainLimited {
		b.WriteString("(deny network*)\n")
		hostPort := strings.TrimPrefix(network.proxyURL, "http://")
		fmt.Fprintf(&b, "(allow network-outbound (remote tcp %q))\n", hostPort)
	} else if boolDefault(cfg.Sandbox.Network.Enabled, false) {
		b.WriteString("(allow network*)\n")
	} else {
		b.WriteString("(deny network*)\n")
	}
	return b.String(), nil
}

func compactPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
