package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
)

// APIClient defines the interface for making requests to external APIs
type APIClient interface {
	Request(ctx context.Context, method, path string, body interface{}) ([]byte, error)
}

// ConfigLoader defines the interface for loading configuration
type ConfigLoader interface {
	LoadConfig() (*Config, error)
	GetSecurityTrailsAPIKey() (string, error)
}

// Config represents the structure of our config.yaml file
type Config struct {
	MCPServers map[string]MCPServerConfig `yaml:"mcpServers"`
}

// MCPServerConfig represents the configuration for each MCP server
type MCPServerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
}

// SecurityTrailsClient is a client for interacting with the SecurityTrails API
type SecurityTrailsClient struct {
	baseURL    string
	apiKey     string
	httpClient HTTPClient
}

// HTTPClient interface allows mocking of http.Client for testing
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTPClient wraps the standard http.Client to implement the HTTPClient interface
type DefaultHTTPClient struct {
	Client *http.Client
}

// Do implements the HTTPClient interface for DefaultHTTPClient
func (c *DefaultHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}

// NewSecurityTrailsClient creates a new client for SecurityTrails API
func NewSecurityTrailsClient(apiKey string) *SecurityTrailsClient {
	return &SecurityTrailsClient{
		baseURL: "https://api.securitytrails.com",
		apiKey:  apiKey,
		httpClient: &DefaultHTTPClient{
			Client: &http.Client{
				Timeout: 30 * time.Second,
			},
		},
	}
}

// API Base URL constants
const (
	SecurityTrailsBaseURL = "https://api.securitytrails.com"
	SecurityTrailsV1URL   = SecurityTrailsBaseURL + "/v1"
	SecurityTrailsV2URL   = SecurityTrailsBaseURL + "/v2"
)

// =====================
// Argument Types
// =====================

// PingArgs is an empty struct as the ping endpoint doesn't require arguments
type PingArgs struct{}

// ListProjectsArgs is an empty struct as the list projects endpoint doesn't require arguments
type ListProjectsArgs struct{}

// SearchAssetsArgs represents the arguments for searching assets in a project
type SearchAssetsArgs struct {
	ProjectID   string                 `json:"project_id" jsonschema:"required,description=The ID of the project to search assets in"`
	Filter      map[string]interface{} `json:"filter" jsonschema:"description=Filter criteria for assets"`
	Enrichments []string               `json:"enrichments,omitempty" jsonschema:"description=Additional fields to include in the response"`
	Limit       int                    `json:"limit,omitempty" jsonschema:"description=The number of assets to return (default 50, max 1000)"`
	Cursor      string                 `json:"cursor,omitempty" jsonschema:"description=Opaque string provided in next_cursor of previous results"`
}

// FindAssetsArgs represents the arguments for finding assets with GET parameters
type FindAssetsArgs struct {
	ProjectID               string   `json:"project_id" jsonschema:"required,description=The ID of the project to find assets in"`
	Cursor                  string   `json:"cursor,omitempty" jsonschema:"description=Opaque string provided in next_cursor of previous results"`
	Limit                   int      `json:"limit,omitempty" jsonschema:"description=The number of assets to return (default 50, max 1000)"`
	SortBy                  string   `json:"sort_by,omitempty" jsonschema:"description=The field to sort by (default is exposure_score)"`
	AssetType               string   `json:"asset_type,omitempty" jsonschema:"description=The type of asset, one of: domain, ip"`
	CustomTags              string   `json:"custom_tags,omitempty" jsonschema:"description=Filter by custom tags placed on your assets"`
	CustomTagsStrict        string   `json:"custom_tags_strict,omitempty" jsonschema:"description=Filter by custom tags (strict version)"`
	AddedToProjectBefore    string   `json:"added_to_project_before,omitempty" jsonschema:"description=Filter on the date (Y-m-d) the asset was added to the project"`
	AddedToProjectAfter     string   `json:"added_to_project_after,omitempty" jsonschema:"description=Filter on the date (Y-m-d) the asset was added to the project"`
	DiscoveredBefore        string   `json:"discovered_before,omitempty" jsonschema:"description=Filter on the date (Y-m-d) the asset was discovered"`
	DiscoveredAfter         string   `json:"discovered_after,omitempty" jsonschema:"description=Filter on the date (Y-m-d) the asset was discovered"`
	Apex                    string   `json:"apex,omitempty" jsonschema:"description=Filter on the apex domain of the assets"`
	ReferencedIP            string   `json:"referenced_ip,omitempty" jsonschema:"description=Filter on a A or CNAME record pointing to the IP address or contained by the CIDR"`
	ReferencedIPBefore      string   `json:"referenced_ip_before,omitempty" jsonschema:"description=Filter on referenced IP with date range criteria"`
	ReferencedIPAfter       string   `json:"referenced_ip_after,omitempty" jsonschema:"description=Filter on referenced IP with date range criteria"`
	HasDNSRecordType        string   `json:"has_dns_record_type,omitempty" jsonschema:"description=Filter for assets that have this DNS record type"`
	DNSResolves             *bool    `json:"dns_resolves,omitempty" jsonschema:"description=Filter for assets that resolve to a valid IP currently"`
	ASN                     int      `json:"asn,omitempty" jsonschema:"description=Filter for assets in the provided ASN"`
	CNAMEReference          string   `json:"cname_reference,omitempty" jsonschema:"description=Filter on a domain that is referenced by a CNAME record"`
	GeoCountryISO           string   `json:"geo_country_iso,omitempty" jsonschema:"description=Filter for assets in the provided ISO country code"`
	IPOwner                 string   `json:"ip_owner,omitempty" jsonschema:"description=Filter for assets owned by the provided organization"`
	WHOISEmail              string   `json:"whois_email,omitempty" jsonschema:"description=Filter for assets where the WHOIS email address matches"`
	WHOISEmailCurrent       string   `json:"whois_email_current,omitempty" jsonschema:"description=Filter for assets where the current WHOIS email matches"`
	OpenPortNumber          int      `json:"open_port_number,omitempty" jsonschema:"description=Filter for assets with the provided port number"`
	OpenPortProtocol        string   `json:"open_port_protocol,omitempty" jsonschema:"description=Filter for assets with an open port on the provided protocol"`
	OpenPortService         string   `json:"open_port_service,omitempty" jsonschema:"description=Filter for assets with a port supporting the provided protocol"`
	OpenPortTechnology      string   `json:"open_port_technology,omitempty" jsonschema:"description=Filter for assets with a specific product on an open port"`
	TechnologyName          string   `json:"technology_name,omitempty" jsonschema:"description=Filter for the name of a technology found on the asset"`
	WebTechnologyName       string   `json:"web_technology_name,omitempty" jsonschema:"description=Filter for the name of a web technology"`
	CertificateIssuer       string   `json:"certificate_issuer,omitempty" jsonschema:"description=Filter where the certificate issuer matches"`
	CertificateExpBefore    string   `json:"certificate_expires_before,omitempty" jsonschema:"description=Filter where the certificate expiration matches the provided value"`
	CertificateExpAfter     string   `json:"certificate_expires_after,omitempty" jsonschema:"description=Filter where the certificate expiration matches the provided value"`
	CertificateIssBefore    string   `json:"certificate_issued_before,omitempty" jsonschema:"description=Filter where the certificate issuance date matches the provided value"`
	CertificateIssAfter     string   `json:"certificate_issued_after,omitempty" jsonschema:"description=Filter where the certificate issuance date matches the provided value"`
	CertificateSubject      string   `json:"certificate_subject,omitempty" jsonschema:"description=Filter where certificate subject or organizationName matches"`
	CertificateSubAltName   string   `json:"certificate_subject_alt_name,omitempty" jsonschema:"description=Filter where the certificate SAN matches"`
	CertificateSHA256       string   `json:"certificate_sha256,omitempty" jsonschema:"description=Filter where the certificate public key sha256 value matches"`
	CertificateCoversDomain string   `json:"certificate_covers_domain,omitempty" jsonschema:"description=Filter where the certificate covers the provided domain"`
	WAFDetected             *bool    `json:"waf_detected,omitempty" jsonschema:"description=Filter for assets where a WAF is detected"`
	WAFName                 string   `json:"waf_name,omitempty" jsonschema:"description=Filter for assets where a specific WAF is detected"`
	ExposureScoreGTE        int      `json:"exposure_score_gte,omitempty" jsonschema:"description=Filter for assets with exposure score greater than or equal"`
	ExposureScoreLTE        int      `json:"exposure_score_lte,omitempty" jsonschema:"description=Filter for assets with exposure score less than or equal"`
	ExposureSeverity        string   `json:"exposure_severity,omitempty" jsonschema:"description=Filter for assets with severity matching or higher"`
	ExposureID              string   `json:"exposure_id,omitempty" jsonschema:"description=Filter for assets which have an exposure with the provided ID"`
	AdditionalFields        []string `json:"additional_fields,omitempty" jsonschema:"description=A list of additional fields to include in the response"`
}

// ReadAssetArgs represents the arguments for reading a specific asset
type ReadAssetArgs struct {
	ProjectID        string   `json:"project_id" jsonschema:"required,description=The ID of the project containing the asset"`
	AssetID          string   `json:"asset_id" jsonschema:"required,description=The ID of the asset to read (IP or domain)"`
	AdditionalFields []string `json:"additional_fields,omitempty" jsonschema:"description=A list of additional fields to include in the response"`
}

// ListAssetExposuresArgs represents the arguments for listing exposures of an asset
type ListAssetExposuresArgs struct {
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project containing the asset"`
	AssetID   string `json:"asset_id" jsonschema:"required,description=The ID of the asset to list exposures for (IP or domain)"`
}

// GetFiltersArgs represents the arguments for getting filters for a project
type GetFiltersArgs struct {
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project to get filters for"`
}

// TagArgs represents common fields for tagging operations
type TagArgs struct {
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project"`
	AssetID   string `json:"asset_id" jsonschema:"required,description=The ID of the asset (IP or domain)"`
	TagName   string `json:"tag_name" jsonschema:"required,description=The name of the tag"`
}

// BulkTagAssetsArgs represents the arguments for bulk tagging assets
type BulkTagAssetsArgs struct {
	ProjectID string                         `json:"project_id" jsonschema:"required,description=The ID of the project"`
	AssetTags map[string]map[string][]string `json:"asset_tags" jsonschema:"required,description=Add/Remove options keyed on the asset"`
}

// AddTagArgs represents the arguments for adding a tag to a project
type AddTagArgs struct {
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project"`
	TagName   string `json:"tag_name" jsonschema:"required,description=The name of the tag to add"`
}

// GetTagsArgs represents the arguments for getting tags in a project
type GetTagsArgs struct {
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project"`
}

// GetTagStatusArgs represents the arguments for getting tag task status
type GetTagStatusArgs struct {
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project"`
	TaskID    string `json:"task_id" jsonschema:"required,description=The ID of the tagging task"`
}

// ListExposuresArgs represents the arguments for listing exposures in a project
type ListExposuresArgs struct {
	ProjectID      string `json:"project_id" jsonschema:"required,description=The ID of the project"`
	Cursor         string `json:"cursor,omitempty" jsonschema:"description=Opaque string provided in next_cursor of previous results"`
	Limit          int    `json:"limit,omitempty" jsonschema:"description=The number of exposures to return (default 100, max 1000)"`
	FilterCVEID    string `json:"filter_cve_id,omitempty" jsonschema:"description=Filter for assets tied to a vulnerability with the provided CVE"`
	FilterSeverity string `json:"filter_severity,omitempty" jsonschema:"description=Filter for assets with exposure severity matching or higher"`
}

// GetExposureAssetsArgs represents the arguments for getting assets with a specific exposure
type GetExposureAssetsArgs struct {
	ProjectID   string `json:"project_id" jsonschema:"required,description=The ID of the project"`
	SignatureID string `json:"signature_id" jsonschema:"required,description=The ID of the exposure signature"`
	Cursor      string `json:"cursor,omitempty" jsonschema:"description=Opaque string provided in next_cursor of previous results"`
	Limit       int    `json:"limit,omitempty" jsonschema:"description=The number of assets to return (default 100, max 1000)"`
}

// =====================
// API Response Types
// =====================

// PingResponse represents the response from the ping endpoint
type PingResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// =====================
// Configuration & Client
// =====================

// APIError represents an error response from the SecurityTrails API
type APIError struct {
	StatusCode int
	Message    string
	Details    map[string]interface{}
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API Error (HTTP %d): %s", e.StatusCode, e.Message)
}

// DefaultConfigLoader implements the ConfigLoader interface
type DefaultConfigLoader struct{}

// LoadConfig loads the configuration from the config.yaml file
func (l *DefaultConfigLoader) LoadConfig() (*Config, error) {
	// Try to find config in different locations
	configPaths := []string{}

	// 1. Try executable directory
	execPath, err := os.Executable()
	if err == nil {
		configPaths = append(configPaths, filepath.Join(filepath.Dir(execPath), "config.yaml"))
	}

	// 2. Try current working directory
	workDir, err := os.Getwd()
	if err == nil {
		configPaths = append(configPaths, filepath.Join(workDir, "config.yaml"))
		configPaths = append(configPaths, filepath.Join(workDir, "dist", "config.yaml"))
	}

	// Try each config path
	var configData []byte
	var configPath string

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if err == nil {
			configData = data
			configPath = path
			break
		}
	}

	if configData == nil {
		return nil, fmt.Errorf("could not find config.yaml in any of the expected locations")
	}

	// Parse the config file
	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	return &config, nil
}

// GetSecurityTrailsAPIKey extracts the SecurityTrails API key from the environment variable or config
func (l *DefaultConfigLoader) GetSecurityTrailsAPIKey() (string, error) {
	// First, check if the API key is available as an environment variable
	apiKey := os.Getenv("SECURITYTRAILS_API_KEY")
	if apiKey != "" {
		return apiKey, nil
	}

	// If not found in environment, try to load from config
	config, err := l.LoadConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	// Extract the API key from the config
	securityTrailsConfig, exists := config.MCPServers["securitytrails"]
	if !exists {
		return "", fmt.Errorf("securitytrails configuration not found in config.yaml")
	}

	apiKey, exists = securityTrailsConfig.Env["SECURITYTRAILS_API_KEY"]
	if !exists || apiKey == "" {
		return "", fmt.Errorf("SecurityTrails API key not found in config.yaml")
	}

	return apiKey, nil
}

// getSecurityTrailsClient returns a client for interacting with the SecurityTrails API
// This is a convenience function that uses the DefaultConfigLoader
func getSecurityTrailsClient() (*SecurityTrailsClient, error) {
	loader := &DefaultConfigLoader{}
	apiKey, err := loader.GetSecurityTrailsAPIKey()
	if err != nil {
		return nil, err
	}
	return NewSecurityTrailsClient(apiKey), nil
}

// Request makes a request to the SecurityTrails API
func (c *SecurityTrailsClient) Request(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader

	// Prepare request body if provided
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Apikey", c.apiKey)

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle non-OK response codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errorDetails map[string]interface{}
		if err := json.Unmarshal(respBody, &errorDetails); err != nil {
			errorDetails = map[string]interface{}{
				"raw_response": string(respBody),
			}
		}

		errMsg := fmt.Sprintf("API returned HTTP %d", resp.StatusCode)
		if msg, ok := errorDetails["message"].(string); ok {
			errMsg = msg
		}

		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    errMsg,
			Details:    errorDetails,
		}
	}

	return respBody, nil
}

// FormatResponse formats API responses for display
func FormatResponse(data []byte) (string, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// addQueryParams adds query parameters to a URL from a map
func addQueryParams(req *http.Request, params map[string][]string) {
	q := req.URL.Query()
	for key, values := range params {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	req.URL.RawQuery = q.Encode()
}

// =====================
// Tool Implementations
// =====================

// ToolDependencies defines the dependencies for tool implementations
type ToolDependencies struct {
	Client APIClient
}

// pingTool implements the ping endpoint for SecurityTrails API
func pingTool(deps ToolDependencies, args PingArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, err := deps.Client.Request(ctx, http.MethodGet, "/v1/ping", nil)
	if err != nil {
		return "", fmt.Errorf("ping request failed: %w", err)
	}

	var pingResp PingResponse
	if err := json.Unmarshal(data, &pingResp); err != nil {
		return "", fmt.Errorf("failed to parse ping response: %w", err)
	}

	response := fmt.Sprintf("SecurityTrails API Ping Response:\nSuccess: %t\nMessage: %s",
		pingResp.Success, pingResp.Message)

	return response, nil
}

// listProjectsTool implements the list projects endpoint
func listProjectsTool(deps ToolDependencies, args ListProjectsArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	data, err := deps.Client.Request(ctx, http.MethodGet, "/v2/projects", nil)
	if err != nil {
		return "", fmt.Errorf("list projects request failed: %w", err)
	}

	return FormatResponse(data)
}

// searchAssetsTool implements the search assets endpoint
func searchAssetsTool(deps ToolDependencies, args SearchAssetsArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Prepare request body
	requestBody := map[string]interface{}{
		"filter":      args.Filter,
		"enrichments": args.Enrichments,
		"pagination": map[string]interface{}{
			"limit":  args.Limit,
			"cursor": args.Cursor,
		},
	}

	path := fmt.Sprintf("/v2/projects/%s/assets/_search", args.ProjectID)
	data, err := deps.Client.Request(ctx, http.MethodPost, path, requestBody)
	if err != nil {
		return "", fmt.Errorf("search assets request failed: %w", err)
	}

	return FormatResponse(data)
}

// findAssetsTool implements the find assets endpoint (GET version)
func findAssetsTool(deps ToolDependencies, args FindAssetsArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Prepare the URL and build query parameters
	path := fmt.Sprintf("/v2/projects/%s/assets", args.ProjectID)

	// Build query parameters
	queryParams := make(map[string][]string)

	// Helper function to add non-empty string params
	addStringParam := func(key, value string) {
		if value != "" {
			queryParams[key] = []string{value}
		}
	}

	// Helper function to add non-zero int params
	addIntParam := func(key string, value int) {
		if value > 0 {
			queryParams[key] = []string{fmt.Sprintf("%d", value)}
		}
	}

	// Helper function to add bool pointer params
	addBoolParam := func(key string, value *bool) {
		if value != nil {
			queryParams[key] = []string{fmt.Sprintf("%t", *value)}
		}
	}

	// Add all query parameters
	addStringParam("cursor", args.Cursor)
	addIntParam("limit", args.Limit)
	addStringParam("sort_by", args.SortBy)
	addStringParam("asset_type", args.AssetType)
	addStringParam("custom_tags", args.CustomTags)
	addStringParam("custom_tags_strict", args.CustomTagsStrict)
	addStringParam("added_to_project_before", args.AddedToProjectBefore)
	addStringParam("added_to_project_after", args.AddedToProjectAfter)
	addStringParam("discovered_before", args.DiscoveredBefore)
	addStringParam("discovered_after", args.DiscoveredAfter)
	addStringParam("apex", args.Apex)
	addStringParam("referenced_ip", args.ReferencedIP)
	addStringParam("referenced_ip_before", args.ReferencedIPBefore)
	addStringParam("referenced_ip_after", args.ReferencedIPAfter)
	addStringParam("has_dns_record_type", args.HasDNSRecordType)
	addBoolParam("dns_resolves", args.DNSResolves)
	addIntParam("asn", args.ASN)
	addStringParam("cname_reference", args.CNAMEReference)
	addStringParam("geo_country_iso", args.GeoCountryISO)
	addStringParam("ip_owner", args.IPOwner)
	addStringParam("whois_email", args.WHOISEmail)
	addStringParam("whois_email_current", args.WHOISEmailCurrent)
	addIntParam("open_port_number", args.OpenPortNumber)
	addStringParam("open_port_protocol", args.OpenPortProtocol)
	addStringParam("open_port_service", args.OpenPortService)
	addStringParam("open_port_technology", args.OpenPortTechnology)
	addStringParam("technology_name", args.TechnologyName)
	addStringParam("web_technology_name", args.WebTechnologyName)
	addStringParam("certificate_issuer", args.CertificateIssuer)
	addStringParam("certificate_expires_before", args.CertificateExpBefore)
	addStringParam("certificate_expires_after", args.CertificateExpAfter)
	addStringParam("certificate_issued_before", args.CertificateIssBefore)
	addStringParam("certificate_issued_after", args.CertificateIssAfter)
	addStringParam("certificate_subject", args.CertificateSubject)
	addStringParam("certificate_subject_alt_name", args.CertificateSubAltName)
	addStringParam("certificate_sha256", args.CertificateSHA256)
	addStringParam("certificate_covers_domain", args.CertificateCoversDomain)
	addBoolParam("waf_detected", args.WAFDetected)
	addStringParam("waf_name", args.WAFName)
	addIntParam("exposure_score_gte", args.ExposureScoreGTE)
	addIntParam("exposure_score_lte", args.ExposureScoreLTE)
	addStringParam("exposure_severity", args.ExposureSeverity)
	addStringParam("exposure_id", args.ExposureID)

	// Add additional fields as array params
	if len(args.AdditionalFields) > 0 {
		queryParams["additional_fields"] = args.AdditionalFields
	}

	// Create a request to build the URL with query parameters
	req, err := http.NewRequest(http.MethodGet, "https://api.securitytrails.com"+path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	addQueryParams(req, queryParams)

	// Extract the path with query parameters
	fullPath := path + "?" + req.URL.RawQuery

	data, err := deps.Client.Request(ctx, http.MethodGet, fullPath, nil)
	if err != nil {
		return "", fmt.Errorf("find assets request failed: %w", err)
	}

	return FormatResponse(data)
}

// readAssetTool implements the read asset endpoint
func readAssetTool(deps ToolDependencies, args ReadAssetArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Prepare the URL and build query parameters
	path := fmt.Sprintf("/v2/projects/%s/assets/%s", args.ProjectID, args.AssetID)

	// Add additional fields if specified
	queryParams := make(map[string][]string)
	if len(args.AdditionalFields) > 0 {
		queryParams["additional_fields"] = args.AdditionalFields
	}

	// Create a request to build the URL with query parameters
	req, err := http.NewRequest(http.MethodGet, "https://api.securitytrails.com"+path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	addQueryParams(req, queryParams)

	// Extract the path with query parameters
	fullPath := path
	if len(req.URL.RawQuery) > 0 {
		fullPath += "?" + req.URL.RawQuery
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, fullPath, nil)
	if err != nil {
		return "", fmt.Errorf("read asset request failed: %w", err)
	}

	return FormatResponse(data)
}

// listAssetExposuresTool implements the list asset exposures endpoint
func listAssetExposuresTool(deps ToolDependencies, args ListAssetExposuresArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := fmt.Sprintf("/v2/projects/%s/assets/%s/exposures", args.ProjectID, args.AssetID)

	data, err := deps.Client.Request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("list asset exposures request failed: %w", err)
	}

	return FormatResponse(data)
}

// getFiltersTool implements the get filters endpoint
func getFiltersTool(deps ToolDependencies, args GetFiltersArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := fmt.Sprintf("/v2/projects/%s/filters", args.ProjectID)

	data, err := deps.Client.Request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("get filters request failed: %w", err)
	}

	return FormatResponse(data)
}

// applyTagToAssetTool implements the apply tag to asset endpoint
func applyTagToAssetTool(deps ToolDependencies, args TagArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := fmt.Sprintf("/v2/projects/%s/assets/%s/tags/%s", args.ProjectID, args.AssetID, args.TagName)

	data, err := deps.Client.Request(ctx, http.MethodPut, path, nil)
	if err != nil {
		return "", fmt.Errorf("apply tag request failed: %w", err)
	}

	return FormatResponse(data)
}

// removeTagFromAssetTool implements the remove tag from asset endpoint
func removeTagFromAssetTool(deps ToolDependencies, args TagArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := fmt.Sprintf("/v2/projects/%s/assets/%s/tags/%s", args.ProjectID, args.AssetID, args.TagName)

	data, err := deps.Client.Request(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return "", fmt.Errorf("remove tag request failed: %w", err)
	}

	return FormatResponse(data)
}

// bulkAddRemoveAssetTagsTool implements the bulk add remove asset tags endpoint
func bulkAddRemoveAssetTagsTool(deps ToolDependencies, args BulkTagAssetsArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	path := fmt.Sprintf("/v2/projects/%s/tags/_bulk_tag_assets", args.ProjectID)

	// Create the request body
	requestBody := map[string]interface{}{
		"asset_tags": args.AssetTags,
	}

	data, err := deps.Client.Request(ctx, http.MethodPost, path, requestBody)
	if err != nil {
		return "", fmt.Errorf("bulk tag assets request failed: %w", err)
	}

	return FormatResponse(data)
}

// getTagsTool implements the get tags endpoint
func getTagsTool(deps ToolDependencies, args GetTagsArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := fmt.Sprintf("/v2/projects/%s/tags", args.ProjectID)

	data, err := deps.Client.Request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("get tags request failed: %w", err)
	}

	return FormatResponse(data)
}

// getTagStatusTool implements the get tag status endpoint
func getTagStatusTool(deps ToolDependencies, args GetTagStatusArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := fmt.Sprintf("/v2/projects/%s/tags/_task_status/%s", args.ProjectID, args.TaskID)

	data, err := deps.Client.Request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("get tag status request failed: %w", err)
	}

	return FormatResponse(data)
}

// addTagTool implements the add tag endpoint
func addTagTool(deps ToolDependencies, args AddTagArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := fmt.Sprintf("/v2/projects/%s/tags/%s", args.ProjectID, args.TagName)

	data, err := deps.Client.Request(ctx, http.MethodPost, path, nil)
	if err != nil {
		return "", fmt.Errorf("add tag request failed: %w", err)
	}

	return FormatResponse(data)
}

// listExposuresTool implements the list exposures endpoint
func listExposuresTool(deps ToolDependencies, args ListExposuresArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Prepare the URL and build query parameters
	path := fmt.Sprintf("/v2/projects/%s/exposures", args.ProjectID)

	// Build query parameters
	queryParams := make(map[string][]string)

	// Helper function to add non-empty string params
	addStringParam := func(key, value string) {
		if value != "" {
			queryParams[key] = []string{value}
		}
	}

	// Helper function to add non-zero int params
	addIntParam := func(key string, value int) {
		if value > 0 {
			queryParams[key] = []string{fmt.Sprintf("%d", value)}
		}
	}

	addStringParam("cursor", args.Cursor)
	addIntParam("limit", args.Limit)
	addStringParam("filter_cve_id", args.FilterCVEID)
	addStringParam("filter_severity", args.FilterSeverity)

	// Create a request to build the URL with query parameters
	req, err := http.NewRequest(http.MethodGet, "https://api.securitytrails.com"+path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	addQueryParams(req, queryParams)

	// Extract the path with query parameters
	fullPath := path
	if len(req.URL.RawQuery) > 0 {
		fullPath += "?" + req.URL.RawQuery
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, fullPath, nil)
	if err != nil {
		return "", fmt.Errorf("list exposures request failed: %w", err)
	}

	return FormatResponse(data)
}

// getExposureAssetsTool implements the get exposure assets endpoint
func getExposureAssetsTool(deps ToolDependencies, args GetExposureAssetsArgs) (string, error) {
	if deps.Client == nil {
		client, err := getSecurityTrailsClient()
		if err != nil {
			return "", fmt.Errorf("failed to get SecurityTrails client: %w", err)
		}
		deps.Client = client
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Prepare the URL and build query parameters
	path := fmt.Sprintf("/v2/projects/%s/exposures/%s", args.ProjectID, args.SignatureID)

	// Build query parameters
	queryParams := make(map[string][]string)

	// Helper function to add non-empty string params
	addStringParam := func(key, value string) {
		if value != "" {
			queryParams[key] = []string{value}
		}
	}

	// Helper function to add non-zero int params
	addIntParam := func(key string, value int) {
		if value > 0 {
			queryParams[key] = []string{fmt.Sprintf("%d", value)}
		}
	}

	addStringParam("cursor", args.Cursor)
	addIntParam("limit", args.Limit)

	// Create a request to build the URL with query parameters
	req, err := http.NewRequest(http.MethodGet, "https://api.securitytrails.com"+path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	addQueryParams(req, queryParams)

	// Extract the path with query parameters
	fullPath := path
	if len(req.URL.RawQuery) > 0 {
		fullPath += "?" + req.URL.RawQuery
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, fullPath, nil)
	if err != nil {
		return "", fmt.Errorf("get exposure assets request failed: %w", err)
	}

	return FormatResponse(data)
}
