package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"manifold/internal/embeddedpg"
)

func runEmbeddedPostgresCommand(args []string, stdout, stderr io.Writer) (bool, int) {
	if len(args) < 2 || args[0] != "embedded-postgres" || args[1] != "doctor" {
		return false, 0
	}
	flags := flag.NewFlagSet("embedded-postgres doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	allowExternal := flags.Bool("allow-external-runtime-resolution", false, "allow development fallback to downloaded/system PostgreSQL runtimes")
	diagnosticLog := flags.String("diagnostic-log", "", "diagnostic log path to include in failures")
	if err := flags.Parse(args[2:]); err != nil {
		return true, 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result := embeddedpg.Doctor(ctx, embeddedpg.DoctorRequest{
		AllowExternalRuntimeResolution: *allowExternal,
		DiagnosticLogPath:              *diagnosticLog,
	})

	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			_, _ = fmt.Fprintf(stderr, "write doctor JSON: %v\n", err)
			return true, 1
		}
	} else if result.OK {
		_, _ = fmt.Fprintf(stdout, "embedded PostgreSQL runtime %s passed doctor checks\n", result.RuntimeID)
	} else {
		_, _ = fmt.Fprintf(stderr, "embedded PostgreSQL doctor failed: %s\n", result.Error)
	}

	if !result.OK {
		return true, 1
	}
	return true, 0
}
