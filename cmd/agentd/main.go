package main

import (
	"os"

	"manifold/internal/agentd"
)

func main() {
	if handled, code := runEmbeddedPostgresCommand(os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	agentd.Run()
}
