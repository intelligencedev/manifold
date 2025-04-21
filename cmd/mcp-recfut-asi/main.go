package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

// RunMCPServer is the main entry point for running the SecurityTrails API MCP server.
func main() {
	log.Println("Starting SecurityTrails API MCP Server...")

	// Create a transport for the server
	serverTransport := stdio.NewStdioServerTransport()

	// Create a new server with the transport
	server := mcp.NewServer(serverTransport)

	// Register all SecurityTrails MCP tools
	registerSecurityTrailsTools(server)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

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
		log.Fatalf("Server error: %v", err)
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down...", sig)
	}

	log.Println("SecurityTrails MCP server stopped")
}

// registerSecurityTrailsTools registers all the tools that our SecurityTrails MCP server will provide
func registerSecurityTrailsTools(server *mcp.Server) {
	// Basic tools
	registerBasicTools(server)

	// Project API tools
	registerProjectTools(server)

	// Asset API tools
	registerAssetTools(server)

	// Tag API tools
	registerTagTools(server)

	// Exposure API tools
	registerExposureTools(server)

	log.Println("All SecurityTrails MCP tools registered successfully")
}

// registerBasicTools registers the basic utility tools
func registerBasicTools(server *mcp.Server) {
	// Register API ping tool
	if err := server.RegisterTool("ping", "Check API status for SecurityTrails", func(args PingArgs) (*mcp.ToolResponse, error) {
		res, err := pingTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering ping tool: %v", err)
	}
}

// registerProjectTools registers the project-related tools
func registerProjectTools(server *mcp.Server) {
	// Register list projects tool
	if err := server.RegisterTool("list_projects", "List all projects the user has access to", func(args ListProjectsArgs) (*mcp.ToolResponse, error) {
		res, err := listProjectsTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering list_projects tool: %v", err)
	}
}

// registerAssetTools registers the asset-related tools
func registerAssetTools(server *mcp.Server) {
	// Register search assets tool
	if err := server.RegisterTool("search_assets", "Search assets by filter criteria using POST endpoint", func(args SearchAssetsArgs) (*mcp.ToolResponse, error) {
		res, err := searchAssetsTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering search_assets tool: %v", err)
	}

	// Register find assets tool
	if err := server.RegisterTool("find_assets", "Find assets using GET endpoint with query parameters", func(args FindAssetsArgs) (*mcp.ToolResponse, error) {
		res, err := findAssetsTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering find_assets tool: %v", err)
	}

	// Register read asset tool
	if err := server.RegisterTool("read_asset", "Read a specific asset by its ID", func(args ReadAssetArgs) (*mcp.ToolResponse, error) {
		res, err := readAssetTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering read_asset tool: %v", err)
	}

	// Register list asset exposures tool
	if err := server.RegisterTool("list_asset_exposures", "List exposures for a specific asset", func(args ListAssetExposuresArgs) (*mcp.ToolResponse, error) {
		res, err := listAssetExposuresTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering list_asset_exposures tool: %v", err)
	}

	// Register get filters tool
	if err := server.RegisterTool("get_filters", "Get available filters for assets in a project", func(args GetFiltersArgs) (*mcp.ToolResponse, error) {
		res, err := getFiltersTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering get_filters tool: %v", err)
	}
}

// registerTagTools registers the tag-related tools
func registerTagTools(server *mcp.Server) {
	// Register apply tag to asset tool
	if err := server.RegisterTool("apply_tag_to_asset", "Apply a tag to a specific asset", func(args TagArgs) (*mcp.ToolResponse, error) {
		res, err := applyTagToAssetTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering apply_tag_to_asset tool: %v", err)
	}

	// Register remove tag from asset tool
	if err := server.RegisterTool("remove_tag_from_asset", "Remove a tag from a specific asset", func(args TagArgs) (*mcp.ToolResponse, error) {
		res, err := removeTagFromAssetTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering remove_tag_from_asset tool: %v", err)
	}

	// Register bulk add/remove asset tags tool
	if err := server.RegisterTool("bulk_tag_assets", "Bulk add/remove tags from multiple assets", func(args BulkTagAssetsArgs) (*mcp.ToolResponse, error) {
		res, err := bulkAddRemoveAssetTagsTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering bulk_tag_assets tool: %v", err)
	}

	// Register get tags tool
	if err := server.RegisterTool("get_tags", "Get all tags in a project", func(args GetTagsArgs) (*mcp.ToolResponse, error) {
		res, err := getTagsTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering get_tags tool: %v", err)
	}

	// Register get tag status tool
	if err := server.RegisterTool("get_tag_status", "Get the status of a tagging task", func(args GetTagStatusArgs) (*mcp.ToolResponse, error) {
		res, err := getTagStatusTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering get_tag_status tool: %v", err)
	}

	// Register add tag tool
	if err := server.RegisterTool("add_tag", "Add a new tag to a project", func(args AddTagArgs) (*mcp.ToolResponse, error) {
		res, err := addTagTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering add_tag tool: %v", err)
	}
}

// registerExposureTools registers the exposure-related tools
func registerExposureTools(server *mcp.Server) {
	// Register list exposures tool
	if err := server.RegisterTool("list_exposures", "List all exposures in a project", func(args ListExposuresArgs) (*mcp.ToolResponse, error) {
		res, err := listExposuresTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering list_exposures tool: %v", err)
	}

	// Register get exposure assets tool
	if err := server.RegisterTool("get_exposure_assets", "Get assets with a specific exposure", func(args GetExposureAssetsArgs) (*mcp.ToolResponse, error) {
		res, err := getExposureAssetsTool(args)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
	}); err != nil {
		log.Printf("Error registering get_exposure_assets tool: %v", err)
	}
}
