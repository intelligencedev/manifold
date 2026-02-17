package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"manifold/internal/apidocs"
)

func main() {
	var (
		outPath        string
		serverURL      string
		authEnabled    bool
		authCookieName string
	)

	flag.StringVar(&outPath, "out", "docs/openapi/openapi.json", "output file path for OpenAPI JSON")
	flag.StringVar(&serverURL, "server", "http://localhost:32180", "server URL inserted into OpenAPI servers")
	flag.BoolVar(&authEnabled, "auth", false, "include session-cookie auth metadata")
	flag.StringVar(&authCookieName, "auth-cookie", "sio_session", "session cookie name used when -auth is enabled")
	flag.Parse()

	doc, err := apidocs.GenerateSpecJSON(apidocs.Options{
		ServerURL:      serverURL,
		AuthEnabled:    authEnabled,
		AuthCookieName: authCookieName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi: generate failed: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "openapi: create output dir failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, doc, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "openapi: write failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("openapi: wrote %s\n", outPath)
}
