package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

func main() {
	log.Println("Starting SecurityTrails API MCP Server...")

	// Create a context that will be canceled when receiving termination signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, initiating shutdown...", sig)
		cancel()
	}()

	if err := run(ctx); err != nil {
		log.Fatalf("Fatal error: %v", err)
	}

	log.Println("SecurityTrails MCP server stopped gracefully")
}

// run is the main entry point for running the SecurityTrails API MCP server.
func run(ctx context.Context) error {
	// Create a transport for the server
	serverTransport := stdio.NewStdioServerTransport()

	// Create a new server with the transport
	server := mcp.NewServer(serverTransport)

	// Get the SecurityTrails client for dependency injection
	client, err := getSecurityTrailsClient()
	if err != nil {
		return fmt.Errorf("failed to initialize SecurityTrails client: %w", err)
	}

	// Create tool dependencies
	deps := ToolDependencies{
		Client: client,
	}

	// Register all SecurityTrails MCP tools
	if err := registerSecurityTrailsTools(server, deps); err != nil {
		return fmt.Errorf("failed to register tools: %w", err)
	}

	// Start the server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := server.Serve(); err != nil {
			errChan <- fmt.Errorf("MCP server error: %w", err)
		}
	}()

	// Wait for termination signal or error
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		// Context was canceled by signal handler
		return nil
	}
}

// registerSecurityTrailsTools registers all tools that our SecurityTrails MCP server will provide.
// Returns an error if any tool registration fails.
func registerSecurityTrailsTools(server *mcp.Server, deps ToolDependencies) error {
	registrationFuncs := []struct {
		name string
		fn   func(*mcp.Server, ToolDependencies) error
	}{
		{"basic tools", registerBasicTools},
		{"project tools", registerProjectTools},
		{"asset tools", registerAssetTools},
		{"tag tools", registerTagTools},
		{"exposure tools", registerExposureTools},
	}

	for _, reg := range registrationFuncs {
		if err := reg.fn(server, deps); err != nil {
			return fmt.Errorf("failed to register %s: %w", reg.name, err)
		}
	}

	log.Println("All SecurityTrails MCP tools registered successfully")
	return nil
}

// registerBasicTools registers the basic utility tools
func registerBasicTools(server *mcp.Server, deps ToolDependencies) error {
	// Register API ping tool
	if err := server.RegisterTool("ping", "Check API status for SecurityTrails", func(args PingArgs) (*mcp.ToolResponse, error) {
		res, err := pingTool(deps, args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		return fmt.Errorf("failed to register ping tool: %w", err)
	}
	return nil
}

// registerProjectTools registers the project-related tools
func registerProjectTools(server *mcp.Server, deps ToolDependencies) error {
	// Register list projects tool
	if err := server.RegisterTool("list_projects", "List all projects the user has access to", func(args ListProjectsArgs) (*mcp.ToolResponse, error) {
		res, err := listProjectsTool(deps, args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		return fmt.Errorf("failed to register list_projects tool: %w", err)
	}
	return nil
}

// registerAssetTools registers the asset-related tools
func registerAssetTools(server *mcp.Server, deps ToolDependencies) error {
	toolRegistrations := []struct {
		name        string
		description string
		handler     interface{}
	}{
		{
			"search_assets",
			"Search assets by filter criteria using POST endpoint",
			func(args SearchAssetsArgs) (*mcp.ToolResponse, error) {
				res, err := searchAssetsTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"find_assets",
			"Find assets using GET endpoint with query parameters",
			func(args FindAssetsArgs) (*mcp.ToolResponse, error) {
				res, err := findAssetsTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"read_asset",
			"Read a specific asset by its ID",
			func(args ReadAssetArgs) (*mcp.ToolResponse, error) {
				res, err := readAssetTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"list_asset_exposures",
			"List exposures for a specific asset",
			func(args ListAssetExposuresArgs) (*mcp.ToolResponse, error) {
				res, err := listAssetExposuresTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"get_filters",
			"Get available filters for assets in a project",
			func(args GetFiltersArgs) (*mcp.ToolResponse, error) {
				res, err := getFiltersTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
	}

	for _, reg := range toolRegistrations {
		if err := server.RegisterTool(reg.name, reg.description, reg.handler); err != nil {
			return fmt.Errorf("failed to register %s tool: %w", reg.name, err)
		}
	}

	return nil
}

// registerTagTools registers the tag-related tools
func registerTagTools(server *mcp.Server, deps ToolDependencies) error {
	toolRegistrations := []struct {
		name        string
		description string
		handler     interface{}
	}{
		{
			"apply_tag_to_asset",
			"Apply a tag to a specific asset",
			func(args TagArgs) (*mcp.ToolResponse, error) {
				res, err := applyTagToAssetTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"remove_tag_from_asset",
			"Remove a tag from a specific asset",
			func(args TagArgs) (*mcp.ToolResponse, error) {
				res, err := removeTagFromAssetTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"bulk_tag_assets",
			"Bulk add/remove tags from multiple assets",
			func(args BulkTagAssetsArgs) (*mcp.ToolResponse, error) {
				res, err := bulkAddRemoveAssetTagsTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"get_tags",
			"Get all tags in a project",
			func(args GetTagsArgs) (*mcp.ToolResponse, error) {
				res, err := getTagsTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"get_tag_status",
			"Get the status of a tagging task",
			func(args GetTagStatusArgs) (*mcp.ToolResponse, error) {
				res, err := getTagStatusTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"add_tag",
			"Add a new tag to a project",
			func(args AddTagArgs) (*mcp.ToolResponse, error) {
				res, err := addTagTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
	}

	for _, reg := range toolRegistrations {
		if err := server.RegisterTool(reg.name, reg.description, reg.handler); err != nil {
			return fmt.Errorf("failed to register %s tool: %w", reg.name, err)
		}
	}

	return nil
}

// registerExposureTools registers the exposure-related tools
func registerExposureTools(server *mcp.Server, deps ToolDependencies) error {
	toolRegistrations := []struct {
		name        string
		description string
		handler     interface{}
	}{
		{
			"list_exposures",
			"List all exposures in a project",
			func(args ListExposuresArgs) (*mcp.ToolResponse, error) {
				res, err := listExposuresTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
		{
			"get_exposure_assets",
			"Get assets with a specific exposure",
			func(args GetExposureAssetsArgs) (*mcp.ToolResponse, error) {
				res, err := getExposureAssetsTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
	}

	for _, reg := range toolRegistrations {
		if err := server.RegisterTool(reg.name, reg.description, reg.handler); err != nil {
			return fmt.Errorf("failed to register %s tool: %w", reg.name, err)
		}
	}

	return nil
}
