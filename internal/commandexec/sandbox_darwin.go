//go:build darwin

package commandexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"manifold/internal/config"
)

var macOSSandboxExecPath = "/usr/bin/sandbox-exec"

func wrapSandbox(cfg config.ExecConfig, workdir, resolvedCommand string, args, env []string, network *sandboxNetwork) (string, []string, bool, error) {
	if !boolDefault(cfg.Sandbox.Enabled, true) {
		return resolvedCommand, append([]string(nil), args...), false, nil
	}
	sandboxExec := macOSSandboxExecPath
	if _, err := os.Stat(sandboxExec); err != nil {
		if network != nil && network.mode == networkModeDomainLimited {
			return "", nil, false, policyDeny("sandbox-exec is required for domain-limited network egress", "", false)
		}
		return "", nil, false, policyDeny("sandbox-exec is required but unavailable", "", false)
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
	workspacePaths := []string{workdir}
	if realWorkdir, err := filepath.EvalSymlinks(workdir); err == nil {
		workspacePaths = append(workspacePaths, realWorkdir)
	}

	readPaths := append(macOSReadOnlyRoots(), workspacePaths...)
	readPaths = append(readPaths, sandboxReadPaths(workdir, cfg.Sandbox.ReadPaths)...)
	if commandDir := filepath.Dir(resolvedCommand); commandDir != "." && commandDir != string(filepath.Separator) {
		readPaths = append(readPaths, commandDir)
		if realCommandDir, err := filepath.EvalSymlinks(commandDir); err == nil {
			readPaths = append(readPaths, realCommandDir)
		}
	}

	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	b.WriteString("(allow process-fork)\n")
	b.WriteString("(allow process-exec)\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach-lookup)\n")
	b.WriteString("(allow signal (target self))\n")
	b.WriteString("(allow file-read-metadata)\n")
	for _, path := range compactPaths(readPaths) {
		fmt.Fprintf(&b, "(allow file-read* (subpath %q))\n", path)
	}
	b.WriteString("(allow file-read* (literal \"/\"))\n")
	for _, path := range compactPaths(workspacePaths) {
		fmt.Fprintf(&b, "(allow file-write* (subpath %q))\n", path)
	}
	b.WriteString("(allow file-read* file-write* (literal \"/dev/null\") (literal \"/dev/random\") (literal \"/dev/urandom\"))\n")
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

func macOSReadOnlyRoots() []string {
	return []string{
		"/usr",
		"/bin",
		"/sbin",
		"/System",
		"/Library",
		"/opt",
		"/private/var/db/dyld",
	}
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
