package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

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

// API Base URL
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
// Tool Implementations
// =====================

// LoadConfig loads the configuration from the config.yaml file
func LoadConfig() (*Config, error) {
	// First, try to find the config.yaml in the same directory as the executable
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	execDir := filepath.Dir(execPath)
	configPath := filepath.Join(execDir, "config.yaml")

	// Check if the config file exists at the executable directory
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// If not, try to find it in the current working directory
		workDir, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current working directory: %w", err)
		}
		configPath = filepath.Join(workDir, "config.yaml")

		// If still not found, try looking in the dist directory
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = filepath.Join(workDir, "dist", "config.yaml")
		}
	}

	// Read the config file
	configData, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse the config file
	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// GetSecurityTrailsAPIKey extracts the SecurityTrails API key from the environment variable or config
func GetSecurityTrailsAPIKey() (string, error) {
	// First, check if the API key is available as an environment variable
	apiKey := os.Getenv("SECURITYTRAILS_API_KEY")
	if apiKey != "" {
		return apiKey, nil
	}

	// If not found in environment, try to load from config
	config, err := LoadConfig()
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

// makeSecurityTrailsRequest makes a request to the SecurityTrails API
func makeSecurityTrailsRequest(method, endpoint string, body io.Reader) (*http.Response, error) {
	// Get the API key
	apiKey, err := GetSecurityTrailsAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get SecurityTrails API key: %w", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Add("accept", "application/json")
	req.Header.Add("content-type", "application/json")
	req.Header.Add("apikey", apiKey)

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// pingTool implements the ping endpoint for SecurityTrails API
func pingTool(_ PingArgs) (string, error) {
	// Get the API key
	apiKey, err := GetSecurityTrailsAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to get SecurityTrails API key: %w", err)
	}

	// Create the HTTP request
	url := SecurityTrailsV1URL + "/ping"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameter for API key
	q := req.URL.Query()
	q.Add("apikey", apiKey)
	req.URL.RawQuery = q.Encode()

	// Add headers
	req.Header.Add("accept", "application/json")

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the response
	var pingResp PingResponse
	if err := json.Unmarshal(body, &pingResp); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	// Return a formatted response
	response := fmt.Sprintf("SecurityTrails API Ping Response:\nSuccess: %t\nMessage: %s",
		pingResp.Success, pingResp.Message)

	return response, nil
}

// listProjectsTool implements the list projects endpoint
func listProjectsTool(_ ListProjectsArgs) (string, error) {
	url := SecurityTrailsV2URL + "/projects"
	resp, err := makeSecurityTrailsRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list projects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to list projects: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// searchAssetsTool implements the search assets endpoint
func searchAssetsTool(args SearchAssetsArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/assets/_search", SecurityTrailsV2URL, args.ProjectID)

	requestBody, err := json.Marshal(map[string]interface{}{
		"filter":      args.Filter,
		"enrichments": args.Enrichments,
		"pagination": map[string]interface{}{
			"limit":  args.Limit,
			"cursor": args.Cursor,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	resp, err := makeSecurityTrailsRequest("POST", url, strings.NewReader(string(requestBody)))
	if err != nil {
		return "", fmt.Errorf("failed to search assets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to search assets: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// findAssetsTool implements the find assets endpoint (GET version)
func findAssetsTool(args FindAssetsArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/assets", SecurityTrailsV2URL, args.ProjectID)

	// Build query parameters
	q := make(map[string][]string)

	if args.Cursor != "" {
		q["cursor"] = []string{args.Cursor}
	}
	if args.Limit > 0 {
		q["limit"] = []string{fmt.Sprintf("%d", args.Limit)}
	}
	if args.SortBy != "" {
		q["sort_by"] = []string{args.SortBy}
	}
	if args.AssetType != "" {
		q["asset_type"] = []string{args.AssetType}
	}
	if args.CustomTags != "" {
		q["custom_tags"] = []string{args.CustomTags}
	}
	if args.CustomTagsStrict != "" {
		q["custom_tags_strict"] = []string{args.CustomTagsStrict}
	}
	if args.AddedToProjectBefore != "" {
		q["added_to_project_before"] = []string{args.AddedToProjectBefore}
	}
	if args.AddedToProjectAfter != "" {
		q["added_to_project_after"] = []string{args.AddedToProjectAfter}
	}
	if args.DiscoveredBefore != "" {
		q["discovered_before"] = []string{args.DiscoveredBefore}
	}
	if args.DiscoveredAfter != "" {
		q["discovered_after"] = []string{args.DiscoveredAfter}
	}
	if args.Apex != "" {
		q["apex"] = []string{args.Apex}
	}
	if args.ReferencedIP != "" {
		q["referenced_ip"] = []string{args.ReferencedIP}
	}
	if args.ReferencedIPBefore != "" {
		q["referenced_ip_before"] = []string{args.ReferencedIPBefore}
	}
	if args.ReferencedIPAfter != "" {
		q["referenced_ip_after"] = []string{args.ReferencedIPAfter}
	}
	if args.HasDNSRecordType != "" {
		q["has_dns_record_type"] = []string{args.HasDNSRecordType}
	}
	if args.DNSResolves != nil {
		q["dns_resolves"] = []string{fmt.Sprintf("%t", *args.DNSResolves)}
	}
	if args.ASN > 0 {
		q["asn"] = []string{fmt.Sprintf("%d", args.ASN)}
	}
	if args.CNAMEReference != "" {
		q["cname_reference"] = []string{args.CNAMEReference}
	}
	if args.GeoCountryISO != "" {
		q["geo_country_iso"] = []string{args.GeoCountryISO}
	}
	if args.IPOwner != "" {
		q["ip_owner"] = []string{args.IPOwner}
	}
	if args.WHOISEmail != "" {
		q["whois_email"] = []string{args.WHOISEmail}
	}
	if args.WHOISEmailCurrent != "" {
		q["whois_email_current"] = []string{args.WHOISEmailCurrent}
	}
	if args.OpenPortNumber > 0 {
		q["open_port_number"] = []string{fmt.Sprintf("%d", args.OpenPortNumber)}
	}
	if args.OpenPortProtocol != "" {
		q["open_port_protocol"] = []string{args.OpenPortProtocol}
	}
	if args.OpenPortService != "" {
		q["open_port_service"] = []string{args.OpenPortService}
	}
	if args.OpenPortTechnology != "" {
		q["open_port_technology"] = []string{args.OpenPortTechnology}
	}
	if args.TechnologyName != "" {
		q["technology_name"] = []string{args.TechnologyName}
	}
	if args.WebTechnologyName != "" {
		q["web_technology_name"] = []string{args.WebTechnologyName}
	}
	if args.CertificateIssuer != "" {
		q["certificate_issuer"] = []string{args.CertificateIssuer}
	}
	if args.CertificateExpBefore != "" {
		q["certificate_expires_before"] = []string{args.CertificateExpBefore}
	}
	if args.CertificateExpAfter != "" {
		q["certificate_expires_after"] = []string{args.CertificateExpAfter}
	}
	if args.CertificateIssBefore != "" {
		q["certificate_issued_before"] = []string{args.CertificateIssBefore}
	}
	if args.CertificateIssAfter != "" {
		q["certificate_issued_after"] = []string{args.CertificateIssAfter}
	}
	if args.CertificateSubject != "" {
		q["certificate_subject"] = []string{args.CertificateSubject}
	}
	if args.CertificateSubAltName != "" {
		q["certificate_subject_alt_name"] = []string{args.CertificateSubAltName}
	}
	if args.CertificateSHA256 != "" {
		q["certificate_sha256"] = []string{args.CertificateSHA256}
	}
	if args.CertificateCoversDomain != "" {
		q["certificate_covers_domain"] = []string{args.CertificateCoversDomain}
	}
	if args.WAFDetected != nil {
		q["waf_detected"] = []string{fmt.Sprintf("%t", *args.WAFDetected)}
	}
	if args.WAFName != "" {
		q["waf_name"] = []string{args.WAFName}
	}
	if args.ExposureScoreGTE > 0 {
		q["exposure_score_gte"] = []string{fmt.Sprintf("%d", args.ExposureScoreGTE)}
	}
	if args.ExposureScoreLTE > 0 {
		q["exposure_score_lte"] = []string{fmt.Sprintf("%d", args.ExposureScoreLTE)}
	}
	if args.ExposureSeverity != "" {
		q["exposure_severity"] = []string{args.ExposureSeverity}
	}
	if args.ExposureID != "" {
		q["exposure_id"] = []string{args.ExposureID}
	}
	if len(args.AdditionalFields) > 0 {
		q["additional_fields"] = args.AdditionalFields
	}

	// Construct the request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameters
	query := req.URL.Query()
	for k, vs := range q {
		for _, v := range vs {
			query.Add(k, v)
		}
	}
	req.URL.RawQuery = query.Encode()

	// Add headers
	apiKey, err := GetSecurityTrailsAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to get SecurityTrails API key: %w", err)
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("apikey", apiKey)

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to find assets: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// readAssetTool implements the read asset endpoint
func readAssetTool(args ReadAssetArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/assets/%s", SecurityTrailsV2URL, args.ProjectID, args.AssetID)

	// Build query parameters
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add additional fields if specified
	if len(args.AdditionalFields) > 0 {
		q := req.URL.Query()
		for _, field := range args.AdditionalFields {
			q.Add("additional_fields", field)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Add headers
	apiKey, err := GetSecurityTrailsAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to get SecurityTrails API key: %w", err)
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("apikey", apiKey)

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to read asset: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// listAssetExposuresTool implements the list asset exposures endpoint
func listAssetExposuresTool(args ListAssetExposuresArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/assets/%s/exposures", SecurityTrailsV2URL, args.ProjectID, args.AssetID)

	resp, err := makeSecurityTrailsRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list asset exposures: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to list asset exposures: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// getFiltersTool implements the get filters endpoint
func getFiltersTool(args GetFiltersArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/filters", SecurityTrailsV2URL, args.ProjectID)

	resp, err := makeSecurityTrailsRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get filters: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get filters: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// applyTagToAssetTool implements the apply tag to asset endpoint
func applyTagToAssetTool(args TagArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/assets/%s/tags/%s", SecurityTrailsV2URL, args.ProjectID, args.AssetID, args.TagName)

	resp, err := makeSecurityTrailsRequest("PUT", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to apply tag to asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to apply tag to asset: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// removeTagFromAssetTool implements the remove tag from asset endpoint
func removeTagFromAssetTool(args TagArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/assets/%s/tags/%s", SecurityTrailsV2URL, args.ProjectID, args.AssetID, args.TagName)

	resp, err := makeSecurityTrailsRequest("DELETE", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to remove tag from asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to remove tag from asset: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// bulkAddRemoveAssetTagsTool implements the bulk add remove asset tags endpoint
func bulkAddRemoveAssetTagsTool(args BulkTagAssetsArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/tags/_bulk_tag_assets", SecurityTrailsV2URL, args.ProjectID)

	// Create the request body
	requestBody, err := json.Marshal(map[string]interface{}{
		"asset_tags": args.AssetTags,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	resp, err := makeSecurityTrailsRequest("POST", url, strings.NewReader(string(requestBody)))
	if err != nil {
		return "", fmt.Errorf("failed to bulk add/remove asset tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to bulk add/remove asset tags: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// getTagsTool implements the get tags endpoint
func getTagsTool(args GetTagsArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/tags", SecurityTrailsV2URL, args.ProjectID)

	resp, err := makeSecurityTrailsRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get tags: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// getTagStatusTool implements the get tag status endpoint
func getTagStatusTool(args GetTagStatusArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/tags/_task_status/%s", SecurityTrailsV2URL, args.ProjectID, args.TaskID)

	resp, err := makeSecurityTrailsRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get tag status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get tag status: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// addTagTool implements the add tag endpoint
func addTagTool(args AddTagArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/tags/%s", SecurityTrailsV2URL, args.ProjectID, args.TagName)

	resp, err := makeSecurityTrailsRequest("POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to add tag: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to add tag: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// listExposuresTool implements the list exposures endpoint
func listExposuresTool(args ListExposuresArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/exposures", SecurityTrailsV2URL, args.ProjectID)

	// Build query parameters
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

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
	req.URL.RawQuery = q.Encode()

	// Add headers
	apiKey, err := GetSecurityTrailsAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to get SecurityTrails API key: %w", err)
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("apikey", apiKey)

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to list exposures: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}

// getExposureAssetsTool implements the get exposure assets endpoint
func getExposureAssetsTool(args GetExposureAssetsArgs) (string, error) {
	url := fmt.Sprintf("%s/projects/%s/exposures/%s", SecurityTrailsV2URL, args.ProjectID, args.SignatureID)

	// Build query parameters
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	if args.Cursor != "" {
		q.Add("cursor", args.Cursor)
	}
	if args.Limit > 0 {
		q.Add("limit", fmt.Sprintf("%d", args.Limit))
	}
	req.URL.RawQuery = q.Encode()

	// Add headers
	apiKey, err := GetSecurityTrailsAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to get SecurityTrails API key: %w", err)
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("apikey", apiKey)

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get exposure assets: HTTP %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	prettifiedJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to prettify JSON: %w", err)
	}

	return string(prettifiedJSON), nil
}
