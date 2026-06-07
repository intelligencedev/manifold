package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"manifold/internal/config"
	sqlitep "manifold/internal/persistence/sqlite"
)

func runStorageCommand(args []string, stdout, stderr io.Writer) (bool, int) {
	if len(args) < 2 || args[0] != "storage" || args[1] != "doctor" {
		return false, 0
	}
	flags := flag.NewFlagSet("storage doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	path := flags.String("path", "", "SQLite database path")
	if err := flags.Parse(args[2:]); err != nil {
		return true, 2
	}

	sqliteCfg := sqlitep.Config{WAL: true}
	if cfg, err := config.Load(); err == nil {
		sqliteCfg = sqlitep.Config{
			Path:          cfg.Databases.SQLite.Path,
			BusyTimeoutMs: cfg.Databases.SQLite.BusyTimeoutMs,
			WAL:           cfg.Databases.SQLite.WAL,
			MaxOpenConns:  cfg.Databases.SQLite.MaxOpenConns,
		}
	}
	if *path != "" {
		sqliteCfg.Path = *path
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result := sqlitep.Doctor(ctx, sqliteCfg)

	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			_, _ = fmt.Fprintf(stderr, "write storage doctor JSON: %v\n", err)
			return true, 1
		}
	} else if result.OK {
		_, _ = fmt.Fprintf(stdout, "SQLite storage at %s passed doctor checks\n", result.Path)
	} else {
		_, _ = fmt.Fprintf(stderr, "SQLite storage doctor failed: %s\n", result.Error)
	}

	if !result.OK {
		return true, 1
	}
	return true, 0
}
