package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
)

// =======================================================
//            CLIENT & CONFIG (unchanged sections)
// =======================================================

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
type PingArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
}

// ListProjectsArgs is an empty struct as the list projects endpoint doesn't require arguments
type ListProjectsArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
}

// SearchAssetsArgs represents the arguments for searching assets in a project
type SearchAssetsArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint             string                 `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID            string                 `json:"project_id" jsonschema:"required,description=The ID of the project to search assets in"`
	AssetProperties      map[string]interface{} `json:"asset_properties,omitempty" jsonschema:"description=Properties of the asset to search for"`
	TechnologyProperties map[string]interface{} `json:"technology_properties,omitempty" jsonschema:"description=Technology properties to filter by"`
	ExposureProperties   map[string]interface{} `json:"exposure_properties,omitempty" jsonschema:"description=Exposure properties to filter by"`
	FilterRaw            map[string]interface{} `json:"filter_raw,omitempty" jsonschema:"description=Raw filter object (use only if you need complete control over filter structure)"`
	Enrichments          []string               `json:"enrichments,omitempty" jsonschema:"description=Additional fields to include in the response"`
	Limit                int                    `json:"limit,omitempty" jsonschema:"description=The number of assets to return (default 50, max 1000)"`
	Cursor               string                 `json:"cursor,omitempty" jsonschema:"description=Opaque string provided in next_cursor of previous results"`
}

// FindAssetsArgs represents the arguments for finding assets with GET parameters
type FindAssetsArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint                string   `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
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
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint         string   `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID        string   `json:"project_id" jsonschema:"required,description=The ID of the project containing the asset"`
	AssetID          string   `json:"asset_id" jsonschema:"required,description=The ID of the asset to read (IP or domain)"`
	AdditionalFields []string `json:"additional_fields,omitempty" jsonschema:"description=A list of additional fields to include in the response"`
}

// ListAssetExposuresArgs represents the arguments for listing exposures of an asset
type ListAssetExposuresArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint  string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project containing the asset"`
	AssetID   string `json:"asset_id" jsonschema:"required,description=The ID of the asset to list exposures for (IP or domain)"`
}

// GetFiltersArgs represents the arguments for getting filters for a project
type GetFiltersArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint  string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project to get filters for"`
}

// TagArgs represents common fields for tagging operations
type TagArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint  string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project"`
	AssetID   string `json:"asset_id" jsonschema:"required,description=The ID of the asset (IP or domain)"`
	TagName   string `json:"tag_name" jsonschema:"required,description=The name of the tag"`
}

// BulkTagAssetsArgs represents the arguments for bulk tagging assets
type BulkTagAssetsArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint  string                         `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID string                         `json:"project_id" jsonschema:"required,description=The ID of the project"`
	AssetTags map[string]map[string][]string `json:"asset_tags" jsonschema:"required,description=Add/Remove options keyed on the asset"`
}

// BulkAddRemoveSingleAssetTagsArgs represents arguments for adding/removing tags for a single asset
type BulkAddRemoveSingleAssetTagsArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint   string   `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID  string   `json:"project_id" jsonschema:"required,description=The ID of the project"`
	AssetID    string   `json:"asset_id" jsonschema:"required,description=The ID of the asset (IP or domain)"`
	AddTags    []string `json:"add_tags,omitempty" jsonschema:"description=List of tag names to apply to the asset"`
	RemoveTags []string `json:"remove_tags,omitempty" jsonschema:"description=List of tag names to remove from the asset"`
}

// AddTagArgs represents the arguments for adding a tag to a project
type AddTagArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint  string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project"`
	TagName   string `json:"tag_name" jsonschema:"required,description=The name of the tag to add"`
}

// GetTagsArgs represents the arguments for getting tags in a project
type GetTagsArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint  string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project"`
}

// GetTagStatusArgs represents the arguments for getting tag task status
type GetTagStatusArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint  string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID string `json:"project_id" jsonschema:"required,description=The ID of the project"`
	TaskID    string `json:"task_id" jsonschema:"required,description=The ID of the tagging task"`
}

// ListExposuresArgs represents the arguments for listing exposures in a project
type ListExposuresArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint       string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
	ProjectID      string `json:"project_id" jsonschema:"required,description=The ID of the project"`
	Cursor         string `json:"cursor,omitempty" jsonschema:"description=Opaque string provided in next_cursor of previous results"`
	Limit          int    `json:"limit,omitempty" jsonschema:"description=The number of exposures to return (default 100, max 1000)"`
	FilterCVEID    string `json:"filter_cve_id,omitempty" jsonschema:"description=Filter for assets tied to a vulnerability with the provided CVE"`
	FilterSeverity string `json:"filter_severity,omitempty" jsonschema:"description=Filter for assets with exposure severity matching or higher"`
}

// GetExposureAssetsArgs represents the arguments for getting assets with a specific exposure
type GetExposureAssetsArgs struct {
	// Endpoint is an optional override for the full URL path (including query string).
	// If non-empty, it will be used instead of the default fmt.Sprintf path.
	Endpoint    string `json:"endpoint,omitempty" jsonschema:"description=Optional full endpoint path override"`
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
	var bodyJSON string

	// Prepare request body if provided
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
		bodyJSON = string(bodyBytes)
	}

	// Log the request details
	log.Printf("SecurityTrails API Request: %s %s", method, c.baseURL+path)
	if body != nil {
		log.Printf("Request Body: %s", bodyJSON)
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Apikey", c.apiKey)

	// Log the request URL with query params if any
	if req.URL.RawQuery != "" {
		log.Printf("Request URL with query params: %s?%s", c.baseURL+path, req.URL.RawQuery)

		// Save the request details to a log file
		logFilePath := filepath.Join("/tmp", "securitytrails_request.log")
		logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("Error opening log file: %v", err)
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		defer logFile.Close()
		logFile.WriteString(fmt.Sprintf("Request: %s %s\n", method, c.baseURL+path))
		logFile.WriteString(fmt.Sprintf("Request Body: %s\n", bodyJSON))
		logFile.WriteString(fmt.Sprintf("Request URL with query params: %s?%s\n", c.baseURL+path, req.URL.RawQuery))
		logFile.WriteString(fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339)))
		logFile.WriteString("========================================\n")
		log.Printf("Request details saved to %s", logFilePath)
	} else {
		log.Printf("Request URL: %s", c.baseURL+path)
		log.Printf("Request Body: %s", bodyJSON)

		// Save the request details to a log file
		logFilePath := filepath.Join("/tmp", "securitytrails_request.log")
		logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("Error opening log file: %v", err)
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		defer logFile.Close()
		logFile.WriteString(fmt.Sprintf("Request: %s %s\n", method, c.baseURL+path))
		logFile.WriteString(fmt.Sprintf("Request Body: %s\n", bodyJSON))
		logFile.WriteString(fmt.Sprintf("Request URL: %s\n", c.baseURL+path))
		logFile.WriteString(fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339)))
		logFile.WriteString("========================================\n")
		log.Printf("Request details saved to %s", logFilePath)
	}

	// Make the request
	startTime := time.Now()
	resp, err := c.httpClient.Do(req)
	requestDuration := time.Since(startTime)

	if err != nil {
		log.Printf("Request failed after %v: %v", requestDuration, err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Log the response details
	log.Printf("Response received in %v - Status Code: %d, Size: %d bytes",
		requestDuration, resp.StatusCode, len(respBody))

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

		log.Printf("API Error: %s, Details: %v", errMsg, errorDetails)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    errMsg,
			Details:    errorDetails,
		}
	}

	return respBody, nil
}

// =======================================================
//        RESPONSE HELPERS (new generic pretty-printer)
// =======================================================

// FormatTypedResponse unmarshals the raw JSON into a concrete Go type
// (from types.go), then pretty-prints that value back to JSON so the
// helper continues to return a string for the MCP TextContent payload.
func FormatTypedResponse[T any](data []byte, v *T) (string, error) {
	if err := json.Unmarshal(data, v); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}
	return string(pretty), nil
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

// =======================================================
//                 TOOL IMPLEMENTATIONS
// =======================================================

// ToolDependencies defines the dependencies for tool implementations
type ToolDependencies struct {
	Client APIClient
}

func pingTool(deps ToolDependencies, args PingArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = "/v1/ping"
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("ping request failed: %w", err)
	}

	var resp PingResponse
	return FormatTypedResponse(data, &resp)
}

// ----------------------- Projects ----------------------

func listProjectsTool(deps ToolDependencies, args ListProjectsArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = "/v2/projects"
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("list projects request failed: %w", err)
	}

	var resp ProjectListResponse
	return FormatTypedResponse(data, &resp)
}

// ----------------------- Assets ------------------------

func searchAssetsTool(deps ToolDependencies, args SearchAssetsArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("/v2/projects/%s/assets/_search", args.ProjectID)
	}

	// Build the request body with the proper structure
	body := map[string]interface{}{}

	// Handle filter structure
	filter := map[string]interface{}{}

	// Use raw filter if provided (for advanced use cases)
	if args.FilterRaw != nil && len(args.FilterRaw) > 0 {
		filter = args.FilterRaw
	} else {
		// Build asset_properties with proper EqFilter structure
		assetProperties := map[string]interface{}{}

		if args.AssetProperties != nil && len(args.AssetProperties) > 0 {
			// Process each key in AssetProperties to ensure proper format
			for key, value := range args.AssetProperties {
				// For asset_id specifically, we need to wrap it in an EqFilter
				if key == "asset_id" {
					assetProperties[key] = map[string]interface{}{
						"eq": value,
					}
				} else {
					// For other properties, use as provided
					assetProperties[key] = value
				}
			}
			filter["asset_properties"] = assetProperties
		}

		if args.TechnologyProperties != nil && len(args.TechnologyProperties) > 0 {
			// For technology properties, we need to ensure proper filter structure
			techProperties := map[string]interface{}{}
			for key, value := range args.TechnologyProperties {
				// Handle special properties that need to be formatted as filters
				if key == "waf_detected" {
					techProperties[key] = map[string]interface{}{
						"eq": value,
					}
				} else {
					techProperties[key] = value
				}
			}
			filter["technology_properties"] = techProperties
		}

		if args.ExposureProperties != nil && len(args.ExposureProperties) > 0 {
			filter["exposure_properties"] = args.ExposureProperties
		}
	}

	// Important: Always include the filter object, even if empty
	body["filter"] = filter

	// Always include pagination with at least the default limit
	pagination := map[string]interface{}{
		"limit": 50, // Default limit if none provided
	}

	if args.Limit > 0 {
		pagination["limit"] = args.Limit
	}

	if args.Cursor != "" {
		pagination["cursor"] = args.Cursor
	}

	// Always include pagination
	body["pagination"] = pagination

	// Add enrichments if provided
	if len(args.Enrichments) > 0 {
		body["enrichments"] = args.Enrichments
	}

	// Debug log for examining the exact JSON payload
	jsonBody, _ := json.MarshalIndent(body, "", "  ")
	log.Printf("Search Assets Request JSON Payload: %s", string(jsonBody))

	data, err := deps.Client.Request(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return "", fmt.Errorf("search assets request failed: %w", err)
	}

	var resp ApiListResponseAsset
	return FormatTypedResponse(data, &resp)
}

func findAssetsTool(deps ToolDependencies, args FindAssetsArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		// build query string exactly as before…
		endpoint = buildFindAssetsPath(args)
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("find assets request failed: %w", err)
	}

	var resp ApiListResponseAsset
	return FormatTypedResponse(data, &resp)
}

func readAssetTool(deps ToolDependencies, args ReadAssetArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = buildReadAssetPath(args)
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("read asset request failed: %w", err)
	}

	var resp AssetResponse
	return FormatTypedResponse(data, &resp)
}

func listAssetExposuresTool(deps ToolDependencies, args ListAssetExposuresArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("/v2/projects/%s/assets/%s/exposures", args.ProjectID, args.AssetID)
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("list asset exposures request failed: %w", err)
	}

	var resp ApiListResponseAssetExposure
	return FormatTypedResponse(data, &resp)
}

// ----------------------- Filters -----------------------

func getFiltersTool(deps ToolDependencies, args GetFiltersArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("/v2/projects/%s/filters", args.ProjectID)
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("get filters request failed: %w", err)
	}

	var resp FiltersResponse
	return FormatTypedResponse(data, &resp)
}

// ------------------- Tagging (all ops) -----------------

func applyTagToAssetTool(deps ToolDependencies, args TagArgs) (string, error) {
	return tagMutation(deps, args.Endpoint, http.MethodPut,
		fmt.Sprintf("/v2/projects/%s/assets/%s/tags/%s", args.ProjectID, args.AssetID, args.TagName))
}

func removeTagFromAssetTool(deps ToolDependencies, args TagArgs) (string, error) {
	return tagMutation(deps, args.Endpoint, http.MethodDelete,
		fmt.Sprintf("/v2/projects/%s/assets/%s/tags/%s", args.ProjectID, args.AssetID, args.TagName))
}

func bulkAddRemoveAssetTagsTool(deps ToolDependencies, args BulkTagAssetsArgs) (string, error) {
	// Create a properly formatted request body for bulk tagging
	body := map[string]interface{}{"asset_tags": args.AssetTags}

	// Debug log for examining the request payload
	jsonBody, _ := json.MarshalIndent(body, "", "  ")
	log.Printf("Bulk Tag Assets Request JSON Payload: %s", string(jsonBody))

	return tagBulkMutation(deps, args.Endpoint, body,
		fmt.Sprintf("/v2/projects/%s/tags/_bulk_tag_assets", args.ProjectID))
}

func bulkAddRemoveSingleAssetTagsTool(deps ToolDependencies, args BulkAddRemoveSingleAssetTagsArgs) (string, error) {
	// Create a properly formatted request body
	body := map[string]interface{}{}

	// Handle add_tags property - ensure it's properly formatted
	if len(args.AddTags) > 0 {
		body["add_tags"] = args.AddTags
	}

	// Handle remove_tags property - ensure it's properly formatted
	if len(args.RemoveTags) > 0 {
		body["remove_tags"] = args.RemoveTags
	}

	// Debug log for examining the request payload
	jsonBody, _ := json.MarshalIndent(body, "", "  ")
	log.Printf("Tag Assets Request JSON Payload: %s", string(jsonBody))

	return tagBulkMutation(deps, args.Endpoint, body,
		fmt.Sprintf("/v2/projects/%s/assets/%s/tags", args.ProjectID, args.AssetID))
}

func addTagTool(deps ToolDependencies, args AddTagArgs) (string, error) {
	return tagMutation(deps, args.Endpoint, http.MethodPost,
		fmt.Sprintf("/v2/projects/%s/tags/%s", args.ProjectID, args.TagName))
}

func getTagsTool(deps ToolDependencies, args GetTagsArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("/v2/projects/%s/tags", args.ProjectID)
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("get tags request failed: %w", err)
	}

	var resp ApiListResponseCustomTagPublic
	return FormatTypedResponse(data, &resp)
}

func getTagStatusTool(deps ToolDependencies, args GetTagStatusArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("/v2/projects/%s/tags/_task_status/%s", args.ProjectID, args.TaskID)
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("get tag status request failed: %w", err)
	}

	var resp AssetTagAPIResponse
	return FormatTypedResponse(data, &resp)
}

// helpers shared by tag ops
func tagMutation(deps ToolDependencies, override string, method string, defaultPath string) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path := override
	if path == "" {
		path = defaultPath
	}

	data, err := deps.Client.Request(ctx, method, path, nil)
	if err != nil {
		return "", err
	}

	var resp AssetTagAPIResponse
	return FormatTypedResponse(data, &resp)
}

func tagBulkMutation(deps ToolDependencies, override string, body map[string]interface{}, defaultPath string) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	path := override
	if path == "" {
		path = defaultPath
	}

	data, err := deps.Client.Request(ctx, http.MethodPost, path, body)
	if err != nil {
		return "", err
	}

	var resp AssetTagAPIResponse
	return FormatTypedResponse(data, &resp)
}

// ------------------- Exposures -------------------------

func listExposuresTool(deps ToolDependencies, args ListExposuresArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = buildListExposuresPath(args)
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("list exposures request failed: %w", err)
	}

	var resp ApiListResponseExposureSummary
	return FormatTypedResponse(data, &resp)
}

func getExposureAssetsTool(deps ToolDependencies, args GetExposureAssetsArgs) (string, error) {
	if deps.Client == nil {
		c, err := getSecurityTrailsClient()
		if err != nil {
			return "", err
		}
		deps.Client = c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endpoint := args.Endpoint
	if endpoint == "" {
		endpoint = buildGetExposureAssetsPath(args)
	}

	data, err := deps.Client.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("get exposure assets request failed: %w", err)
	}

	var resp ExposureAssetsListResponse
	return FormatTypedResponse(data, &resp)
}

// =======================================================
//        SMALL HELPERS FOR PATH / QUERY CONSTRUCTION
// =======================================================

// (These helpers keep the main functions clean and were lifted out of
//  the original logic unchanged, except for returning the final path.)

func buildFindAssetsPath(args FindAssetsArgs) string {
	// original build logic retained…
	path := fmt.Sprintf("/v2/projects/%s/assets", args.ProjectID)
	req, _ := http.NewRequest(http.MethodGet, "https://api.securitytrails.com"+path, nil)
	query := req.URL.Query()

	add := func(k, v string) {
		if v != "" {
			query.Add(k, v)
		}
	}
	addInt := func(k string, v int) {
		if v > 0 {
			query.Add(k, fmt.Sprintf("%d", v))
		}
	}
	addBool := func(k string, v *bool) {
		if v != nil {
			query.Add(k, fmt.Sprintf("%t", *v))
		}
	}

	add("cursor", args.Cursor)
	addInt("limit", args.Limit)
	add("sort_by", args.SortBy)

	add("asset_type", args.AssetType)
	add("custom_tags", args.CustomTags)
	add("custom_tags_strict", args.CustomTagsStrict)
	add("added_to_project_before", args.AddedToProjectBefore)
	add("added_to_project_after", args.AddedToProjectAfter)
	add("discovered_before", args.DiscoveredBefore)
	add("discovered_after", args.DiscoveredAfter)
	add("apex", args.Apex)
	add("referenced_ip", args.ReferencedIP)
	add("referenced_ip_before", args.ReferencedIPBefore)
	add("referenced_ip_after", args.ReferencedIPAfter)
	add("has_dns_record_type", args.HasDNSRecordType)
	addBool("dns_resolves", args.DNSResolves)
	addInt("asn", args.ASN)
	add("cname_reference", args.CNAMEReference)
	add("geo_country_iso", args.GeoCountryISO)
	add("ip_owner", args.IPOwner)
	add("whois_email", args.WHOISEmail)
	add("whois_email_current", args.WHOISEmailCurrent)
	addInt("open_port_number", args.OpenPortNumber)
	add("open_port_protocol", args.OpenPortProtocol)
	add("open_port_service", args.OpenPortService)
	add("open_port_technology", args.OpenPortTechnology)
	add("technology_name", args.TechnologyName)
	add("web_technology_name", args.WebTechnologyName)
	add("certificate_issuer", args.CertificateIssuer)
	add("certificate_expires_before", args.CertificateExpBefore)
	add("certificate_expires_after", args.CertificateExpAfter)
	add("certificate_issued_before", args.CertificateIssBefore)
	add("certificate_issued_after", args.CertificateIssAfter)
	add("certificate_subject", args.CertificateSubject)
	add("certificate_subject_alt_name", args.CertificateSubAltName)
	add("certificate_sha256", args.CertificateSHA256)
	add("certificate_covers_domain", args.CertificateCoversDomain)
	addBool("waf_detected", args.WAFDetected)
	add("waf_name", args.WAFName)
	addInt("exposure_score_gte", args.ExposureScoreGTE)
	addInt("exposure_score_lte", args.ExposureScoreLTE)
	add("exposure_severity", args.ExposureSeverity)
	add("exposure_id", args.ExposureID)

	// additional fields
	for _, f := range args.AdditionalFields {
		query.Add("additional_fields", f)
	}

	req.URL.RawQuery = query.Encode()
	if req.URL.RawQuery != "" {
		return path + "?" + req.URL.RawQuery
	}
	return path
}

func buildReadAssetPath(args ReadAssetArgs) string {
	path := fmt.Sprintf("/v2/projects/%s/assets/%s", args.ProjectID, args.AssetID)
	if len(args.AdditionalFields) == 0 {
		return path
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.securitytrails.com"+path, nil)
	for _, f := range args.AdditionalFields {
		req.URL.Query().Add("additional_fields", f)
	}
	return path + "?" + req.URL.Query().Encode()
}

func buildListExposuresPath(args ListExposuresArgs) string {
	path := fmt.Sprintf("/v2/projects/%s/exposures", args.ProjectID)
	req, _ := http.NewRequest(http.MethodGet, "https://api.securitytrails.com"+path, nil)
	q := req.URL.Query()
	if args.Cursor != "" {
		q.Add("cursor", args.Cursor)
	}
	if args.Limit > 0 {
		q.Add("limit", fmt.Sprintf("%d", args.Limit))
	}
	if args.FilterCVEID != "" {
		q.Add("filter_cve_id", args.FilterCVEID)
	}
	if args.FilterSeverity != "" {
		q.Add("filter_severity", args.FilterSeverity)
	}
	if q.Encode() != "" {
		return path + "?" + q.Encode()
	}
	return path
}

func buildGetExposureAssetsPath(args GetExposureAssetsArgs) string {
	path := fmt.Sprintf("/v2/projects/%s/exposures/%s", args.ProjectID, args.SignatureID)
	req, _ := http.NewRequest(http.MethodGet, "https://api.securitytrails.com"+path, nil)
	q := req.URL.Query()
	if args.Cursor != "" {
		q.Add("cursor", args.Cursor)
	}
	if args.Limit > 0 {
		q.Add("limit", fmt.Sprintf("%d", args.Limit))
	}
	if q.Encode() != "" {
		return path + "?" + q.Encode()
	}
	return path
}
