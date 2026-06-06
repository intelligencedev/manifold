package main

import (
	"os"

	"manifold/internal/agentd"
	"manifold/internal/egress"
)

func main() {
	if handled, code := egress.RunSupervisorCommand(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	if handled, code := runEmbeddedPostgresCommand(os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	agentd.Run()
}
