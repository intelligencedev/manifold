package egress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
)

const SupervisorFlag = "--manifold-egress-supervisor"

type SupervisorSpec struct {
	Command          string   `json:"command"`
	Args             []string `json:"args"`
	Env              []string `json:"env"`
	Dir              string   `json:"dir"`
	BrokerSocketPath string   `json:"broker_socket_path"`
	ListenAddress    string   `json:"listen_address"`
}

func RunSupervisorCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, int) {
	if len(args) != 2 || args[0] != SupervisorFlag {
		return false, 0
	}
	code, err := runSupervisor(args[1], stdin, stdout, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "egress supervisor failed: %v\n", err)
		return true, 1
	}
	return true, code
}

func runSupervisor(specPath string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return 1, fmt.Errorf("read supervisor spec: %w", err)
	}
	var spec SupervisorSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return 1, fmt.Errorf("parse supervisor spec: %w", err)
	}
	if spec.Command == "" || spec.BrokerSocketPath == "" {
		return 1, errors.New("supervisor spec is incomplete")
	}
	listenAddr := spec.ListenAddress
	if listenAddr == "" {
		listenAddr = "127.0.0.1:3128"
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return 1, fmt.Errorf("start in-sandbox proxy shim: %w", err)
	}
	defer ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		acceptProxyShim(ctx, ln, spec.BrokerSocketPath)
	}()

	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	cancel()
	_ = ln.Close()
	wg.Wait()
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	}
	return 1, err
}

func acceptProxyShim(ctx context.Context, ln net.Listener, socketPath string) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleProxyShimConn(ctx, conn, socketPath)
	}
}

func handleProxyShimConn(ctx context.Context, conn net.Conn, socketPath string) {
	defer conn.Close()
	upstream, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		return
	}
	defer upstream.Close()
	errCh := make(chan error, 2)
	go proxyCopy(errCh, upstream, conn)
	go proxyCopy(errCh, conn, upstream)
	<-errCh
}
