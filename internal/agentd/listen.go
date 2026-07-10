package agentd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"

	"manifold/internal/config"
)

const preferredListenPort = config.PreferPort

// listenHTTP binds the preferred port and falls back to a free port when busy.
// Returns the listener and the actual bound address (host:port).
func listenHTTP(preferPort int) (net.Listener, string, error) {
	if preferPort <= 0 {
		preferPort = preferredListenPort
	}
	addr := fmt.Sprintf(":%d", preferPort)
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, ln.Addr().String(), nil
	}
	if !isAddrInUse(err) {
		return nil, "", err
	}
	log.Warn().Int("port", preferPort).Msg("preferred port busy; binding free port")
	ln, err = net.Listen("tcp", ":0")
	if err != nil {
		return nil, "", err
	}
	return ln, ln.Addr().String(), nil
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	if op, ok := err.(*net.OpError); ok {
		err = op.Err
	}
	if sys, ok := err.(*os.SyscallError); ok {
		err = sys.Err
	}
	return err == syscall.EADDRINUSE
}

func publicBaseURL(bindAddr string) string {
	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return "http://localhost:" + strconv.Itoa(preferredListenPort)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func findConfigYAMLPath() string {
	if override := strings.TrimSpace(os.Getenv(config.EnvConfigPath)); override != "" {
		return override
	}
	path, found, err := config.ResolveConfigPath()
	if err == nil && found {
		return path
	}
	if path != "" {
		return path
	}
	return config.DefaultConfigPath()
}

func ensureConfigParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
