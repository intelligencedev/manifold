package main

import (
	"fmt"

	mcp "github.com/metoro-io/mcp-golang"
)

// registerBasicTools registers utility endpoints such as ping.
func registerBasicTools(server *mcp.Server, deps ToolDependencies) error {
	if err := server.RegisterTool(
		"ping",
		"Check API status for SecurityTrails",
		func(args PingArgs) (*mcp.ToolResponse, error) {
			res, err := pingTool(deps, args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		},
	); err != nil {
		return fmt.Errorf("failed to register ping tool: %w", err)
	}
	return nil
}

// registerProjectTools registers endpoints that operate on projects.
func registerProjectTools(server *mcp.Server, deps ToolDependencies) error {
	if err := server.RegisterTool(
		"list_projects",
		"List all projects the user has access to",
		func(args ListProjectsArgs) (*mcp.ToolResponse, error) {
			res, err := listProjectsTool(deps, args)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
		},
	); err != nil {
		return fmt.Errorf("failed to register list_projects tool: %w", err)
	}
	return nil
}

// registerAssetTools registers endpoints that read / mutate assets.
func registerAssetTools(server *mcp.Server, deps ToolDependencies) error {
	registrations := []struct {
		name        string
		description string
		handler     interface{}
	}{
		{
			"search_assets",
			"Search assets by filter criteria (POST)",
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
			"Find assets with query parameters (GET)",
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
			"Retrieve a single asset by ID",
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
			"List exposures on a specific asset",
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
		{
			"bulk_add_remove_single_asset_tags",
			"Add / remove multiple tags on a single asset",
			func(args BulkAddRemoveSingleAssetTagsArgs) (*mcp.ToolResponse, error) {
				res, err := bulkAddRemoveSingleAssetTagsTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
	}

	for _, r := range registrations {
		if err := server.RegisterTool(r.name, r.description, r.handler); err != nil {
			return fmt.Errorf("failed to register %s tool: %w", r.name, err)
		}
	}
	return nil
}

// registerTagTools wires endpoints that manage custom tags.
func registerTagTools(server *mcp.Server, deps ToolDependencies) error {
	registrations := []struct {
		name        string
		description string
		handler     interface{}
	}{
		{
			"apply_tag_to_asset",
			"Apply a tag to a single asset",
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
			"Remove a tag from a single asset",
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
			"Bulk add / remove tags across many assets",
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
			"List all tags in a project",
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
			"Retrieve the status of a tag operation",
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
			"Create a new tag in a project",
			func(args AddTagArgs) (*mcp.ToolResponse, error) {
				res, err := addTagTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
	}

	for _, r := range registrations {
		if err := server.RegisterTool(r.name, r.description, r.handler); err != nil {
			return fmt.Errorf("failed to register %s tool: %w", r.name, err)
		}
	}
	return nil
}

// registerExposureTools wires endpoints related to exposure signatures.
func registerExposureTools(server *mcp.Server, deps ToolDependencies) error {
	registrations := []struct {
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
			"Get assets impacted by a specific exposure signature",
			func(args GetExposureAssetsArgs) (*mcp.ToolResponse, error) {
				res, err := getExposureAssetsTool(deps, args)
				if err != nil {
					return nil, err
				}
				return mcp.NewToolResponse(mcp.NewTextContent(res)), nil
			},
		},
	}

	for _, r := range registrations {
		if err := server.RegisterTool(r.name, r.description, r.handler); err != nil {
			return fmt.Errorf("failed to register %s tool: %w", r.name, err)
		}
	}
	return nil
}
