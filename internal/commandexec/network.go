package commandexec

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"manifold/internal/config"
	"manifold/internal/egress"
)

const (
	networkModeNone          = ""
	networkModeDomainLimited = "domain-limited"
)

type sandboxNetwork struct {
	mode          string
	proxyURL      string
	brokerSocket  string
	listenAddress string
	cleanup       func()
	cleanupFiles  []string
}

func prepareSandboxNetwork(cfg config.ExecConfig, workdir string, env []string) (sandboxNetwork, []string, error) {
	if !boolDefault(cfg.Sandbox.Network.Enabled, false) || len(cfg.Sandbox.Network.AllowedDomains) == 0 {
		return sandboxNetwork{mode: networkModeNone}, env, nil
	}
	if !boolDefault(cfg.Sandbox.Enabled, true) {
		return sandboxNetwork{}, nil, policyDeny("allowedDomains requires exec.sandbox.enabled=true", "", false)
	}
	if _, err := egress.NewDomainAllowlist(cfg.Sandbox.Network.AllowedDomains); err != nil {
		return sandboxNetwork{}, nil, policyDeny(err.Error(), "", false)
	}
	switch runtime.GOOS {
	case "darwin":
		return prepareDarwinDomainNetwork(cfg, env)
	case "linux":
		return prepareLinuxDomainNetwork(cfg, workdir, env)
	default:
		return sandboxNetwork{}, nil, policyDeny("sandbox network domain allowlists are not supported on "+runtime.GOOS, "", false)
	}
}

func prepareDarwinDomainNetwork(cfg config.ExecConfig, env []string) (sandboxNetwork, []string, error) {
	broker, err := egress.StartBroker(egress.BrokerOptions{
		AllowedDomains: cfg.Sandbox.Network.AllowedDomains,
		Network:        "tcp",
		Address:        "localhost:0",
	})
	if err != nil {
		return sandboxNetwork{}, nil, policyDeny("start egress broker: "+err.Error(), "", false)
	}
	proxyURL, err := localhostProxyURL(broker.ProxyURL())
	if err != nil {
		_ = broker.Close()
		return sandboxNetwork{}, nil, policyDeny("start egress broker: "+err.Error(), "", false)
	}
	network := sandboxNetwork{
		mode:     networkModeDomainLimited,
		proxyURL: proxyURL,
		cleanup:  func() { _ = broker.Close() },
	}
	return network, withProxyEnv(env, proxyURL), nil
}

func prepareLinuxDomainNetwork(cfg config.ExecConfig, workdir string, env []string) (sandboxNetwork, []string, error) {
	socketPath, err := egressSocketPath(workdir)
	if err != nil {
		return sandboxNetwork{}, nil, err
	}
	broker, err := egress.StartBroker(egress.BrokerOptions{
		AllowedDomains: cfg.Sandbox.Network.AllowedDomains,
		Network:        "unix",
		SocketPath:     socketPath,
	})
	if err != nil {
		return sandboxNetwork{}, nil, policyDeny("start egress broker: "+err.Error(), "", false)
	}
	proxyURL := "http://127.0.0.1:3128"
	network := sandboxNetwork{
		mode:          networkModeDomainLimited,
		proxyURL:      proxyURL,
		brokerSocket:  socketPath,
		listenAddress: "127.0.0.1:3128",
		cleanup: func() {
			_ = broker.Close()
			_ = os.Remove(socketPath)
		},
	}
	return network, withProxyEnv(env, proxyURL), nil
}

func egressSocketPath(workdir string) (string, error) {
	socketDir := filepath.Join(workdir, ".tmp")
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		return "", fmt.Errorf("create egress socket dir: %w", err)
	}
	file, err := os.CreateTemp(socketDir, "egress-*.sock")
	if err != nil {
		return "", fmt.Errorf("create egress socket path: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func withProxyEnv(env []string, proxyURL string) []string {
	out := withoutEnvKeys(env, "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy", "NO_PROXY", "no_proxy", "GIT_HTTP_PROXY", "GIT_HTTPS_PROXY", "GIT_SSH_COMMAND")
	return append(out,
		"HTTP_PROXY="+proxyURL,
		"HTTPS_PROXY="+proxyURL,
		"ALL_PROXY="+proxyURL,
		"http_proxy="+proxyURL,
		"https_proxy="+proxyURL,
		"all_proxy="+proxyURL,
		"NO_PROXY=localhost,127.0.0.1,::1",
		"no_proxy=localhost,127.0.0.1,::1",
		"GIT_HTTP_PROXY="+proxyURL,
		"GIT_HTTPS_PROXY="+proxyURL,
		"GIT_SSH_COMMAND=sh -c 'exit 1'",
	)
}

func withoutEnvKeys(env []string, keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	out := make([]string, 0, len(env))
	for _, item := range env {
		name := item
		if idx := strings.IndexByte(item, '='); idx >= 0 {
			name = item[:idx]
		}
		if _, ok := blocked[name]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func runCleanup(cleanup func()) {
	if cleanup != nil {
		cleanup()
	}
}

func localhostProxyURL(proxyURL string) (string, error) {
	hostPort := strings.TrimPrefix(proxyURL, "http://")
	_, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", err
	}
	return "http://localhost:" + port, nil
}
