package main

import (
	"time"
)

// ApiCount represents the count information in API responses.
type ApiCount struct {
	Total    *int `json:"total,omitempty"` // Total number of items available (may be null if not computable)
	Returned int  `json:"returned"`        // Number of items returned in the current response
}

// PaginationResponse represents pagination details in API responses.
type PaginationResponse struct {
	NextCursor *string      `json:"next_cursor,omitempty"` // Opaque value for retrieving the next page. Null if no more pages.
	Limit      int          `json:"limit,omitempty"`       // The number of items requested per page. Default: 50, Max: 1000
	Total      *int         `json:"total,omitempty"`       // Total number of items matching the query (may be null).
	Sort       *[][2]string `json:"sort,omitempty"`        // The sort order used for the results, e.g., [["field", "asc"]]
}

// ApiMeta holds metadata common to API list responses.
type ApiMeta struct {
	Params     map[string]interface{} `json:"params,omitempty"`     // Parameters used for the request (may be null)
	Counts     *ApiCount              `json:"counts,omitempty"`     // Count information (may be null)
	Pagination *PaginationResponse    `json:"pagination,omitempty"` // Pagination information (may be null)
}

// Project represents a project within the ASI system.
type Project struct {
	ID               string     `json:"id"` // UUID format
	Title            string     `json:"title"`
	ScanningEnabled  *bool      `json:"scanning_enabled,omitempty"`   // Is scanning enabled for this project?
	LastScannedAt    *time.Time `json:"last_scanned_at,omitempty"`    // Format: date-time
	InsertedAt       *time.Time `json:"inserted_at,omitempty"`        // Format: date-time, When the project was created
	MaxExposureScore *int       `json:"max_exposure_score,omitempty"` // Highest exposure score among assets in the project
}

// ProjectListResponse is the response structure for listing projects.
type ProjectListResponse struct {
	Data []Project `json:"data"`
	Meta ApiMeta   `json:"meta"`
}

// AssetSortField represents the valid fields to sort assets by.
// ENUM(discovered_at, added_to_project_at, last_scanned_at, exposure_score, asset_id)
type AssetSortField string

const (
	AssetSortFieldDiscoveredAt     AssetSortField = "discovered_at"
	AssetSortFieldAddedToProjectAt AssetSortField = "added_to_project_at"
	AssetSortFieldLastScannedAt    AssetSortField = "last_scanned_at"
	AssetSortFieldExposureScore    AssetSortField = "exposure_score"
	AssetSortFieldAssetID          AssetSortField = "asset_id"
)

// AssetEnrichment represents additional data fields that can be requested for an asset.
// ENUM(custom_tags, dns_records, whois, ip_metadata, open_tcp_ports, open_udp_ports, web_technologies, certificates, certificate_chain, defenses, exposures, exposure_instance_details)
type AssetEnrichment string

const (
	AssetEnrichmentCustomTags              AssetEnrichment = "custom_tags"
	AssetEnrichmentDNSRecords              AssetEnrichment = "dns_records"
	AssetEnrichmentWHOIS                   AssetEnrichment = "whois"
	AssetEnrichmentIPMetadata              AssetEnrichment = "ip_metadata"
	AssetEnrichmentOpenTCPPorts            AssetEnrichment = "open_tcp_ports"
	AssetEnrichmentOpenUDPPorts            AssetEnrichment = "open_udp_ports"
	AssetEnrichmentWebTechnologies         AssetEnrichment = "web_technologies"
	AssetEnrichmentCertificates            AssetEnrichment = "certificates"
	AssetEnrichmentCertificateChain        AssetEnrichment = "certificate_chain"
	AssetEnrichmentDefenses                AssetEnrichment = "defenses"
	AssetEnrichmentExposures               AssetEnrichment = "exposures"
	AssetEnrichmentExposureInstanceDetails AssetEnrichment = "exposure_instance_details"
)

// EqFilter represents an equality filter.
type EqFilter struct {
	// Can be string, int, ipvanyaddress, ipvanynetwork, date
	Eq interface{} `json:"eq"`
}

// InFilter represents an 'in' filter (value must be one of the provided items).
type InFilter struct {
	// Can be slice of: string, int, ipvanyaddress, ipvanynetwork, date, ExposureSeverity
	In []interface{} `json:"in"`
}

// ContainsFilter represents a substring containment filter.
type ContainsFilter struct {
	Contains string `json:"contains"`
}

// NeqFilter represents a not-equal filter.
type NeqFilter struct {
	// Can be string, int, ipvanyaddress, ipvanynetwork, date
	Neq interface{} `json:"neq"`
}

// DateRangeFilter represents a filter for a date range.
type DateRangeFilter struct {
	Start *string `json:"start,omitempty"` // Format: date (e.g., "YYYY-MM-DD")
	End   *string `json:"end,omitempty"`   // Format: date (e.g., "YYYY-MM-DD")
}

// RequireAllFilter represents a filter where all provided items must match.
type RequireAllFilter struct {
	// Can be slice of: string, int, ipvanyaddress, ipvanynetwork, date, ExposureSeverity
	In []interface{} `json:"in"` // Note: Schema calls this 'in', but behavior is 'require all'
}

// IntEqFilter represents an integer equality filter.
type IntEqFilter struct {
	Eq int `json:"eq"`
}

// IntInFilter represents an integer 'in' filter.
type IntInFilter struct {
	In []int `json:"in"`
}

// EmailEqFilter represents an email equality filter.
type EmailEqFilter struct {
	Eq string `json:"eq"` // Format: email
}

// EmailInFilter represents an email 'in' filter.
type EmailInFilter struct {
	In []string `json:"in"` // Format: email
}

// IntRangeFilter represents a filter for an integer range.
type IntRangeFilter struct {
	Start *int `json:"start"` // Required, but can be null
	End   *int `json:"end"`   // Required, but can be null
}

// BooleanFilter represents a boolean equality filter.
type BooleanFilter struct {
	Eq bool `json:"eq"`
}

// AssetPropertiesFilter defines filters based on asset or DNS properties.
type AssetPropertiesFilter struct {
	// Filter for the specific asset ID (IP or domain).
	AssetID *EqFilter `json:"asset_id,omitempty"`
	// Filter on the apex domain (e.g., "example.com"). Can be EqFilter or InFilter.
	Apex interface{} `json:"apex,omitempty"` // *EqFilter | *InFilter
	// Filter on the date the asset was added to the project.
	AddedToProject *DateRangeFilter `json:"added_to_project,omitempty"`
	// Filter on the date the asset was discovered by ASI.
	Discovered *DateRangeFilter `json:"discovered,omitempty"`
	// Filter by asset type ("domain" or "ip").
	AssetType *EqFilter `json:"asset_type,omitempty"`
	// Filter on A/CNAME records pointing to an IP or CIDR. Can be EqFilter or InFilter.
	ReferencedIP interface{} `json:"referenced_ip,omitempty"` // *EqFilter | *InFilter
	// Filter on a domain referenced by a CNAME record (wildcard match). Can be ContainsFilter or EqFilter.
	CnameReference interface{} `json:"cname_reference,omitempty"` // *ContainsFilter | *EqFilter
	// Filter on the date range a referenced_ip record existed.
	ReferencedIPAt *DateRangeFilter `json:"referenced_ip_at,omitempty"`
	// Filter for assets having a specific DNS record type (e.g., "A", "CNAME"). Can be EqFilter, InFilter, or NeqFilter.
	ValidRecordType interface{} `json:"valid_record_type,omitempty"` // *EqFilter | *InFilter | *NeqFilter
	// Filter by custom tags. Can be EqFilter, InFilter, or RequireAllFilter.
	CustomTags interface{} `json:"custom_tags,omitempty"` // *EqFilter | *InFilter | *RequireAllFilter
	// Strict version of custom_tags filter (validates tag existence). Can be EqFilter, InFilter, or RequireAllFilter.
	CustomTagsStrict interface{} `json:"custom_tags_strict,omitempty"` // *EqFilter | *InFilter | *RequireAllFilter
	// Filter by ASN. Can be IntEqFilter or IntInFilter.
	ASN interface{} `json:"asn,omitempty"` // *IntEqFilter | *IntInFilter
	// Filter by IP geolocation country ISO code (e.g., "US"). Can be EqFilter or InFilter.
	IPGeoCountryISO interface{} `json:"ip_geo_country_iso,omitempty"` // *EqFilter | *InFilter
	// Filter by IP owner organization name. Can be EqFilter or InFilter.
	IPOwner interface{} `json:"ip_owner,omitempty"` // *EqFilter | *InFilter
	// Filter by current WHOIS email. Can be EmailEqFilter or EmailInFilter.
	WhoisEmailCurrent interface{} `json:"whois_email_current,omitempty"` // *EmailEqFilter | *EmailInFilter
	// Filter by historical or current WHOIS email. Can be EmailEqFilter or EmailInFilter.
	WhoisEmail interface{} `json:"whois_email,omitempty"` // *EmailEqFilter | *EmailInFilter
}

// CertificatePropertiesFilter defines filters based on certificate properties.
type CertificatePropertiesFilter struct {
	// Filter by certificate subject common name or organization. Can be EqFilter, InFilter, ContainsFilter, or null.
	CertificateSubject interface{} `json:"certificate_subject,omitempty"` // *EqFilter | *InFilter | *ContainsFilter | nil
	// Filter by certificate Subject Alternate Names. Can be EqFilter, InFilter, ContainsFilter, or null.
	CertificateSubjectAltName interface{} `json:"certificate_subject_alt_name,omitempty"` // *EqFilter | *InFilter | *ContainsFilter | nil
	// Filter by certificate SHA256 hash. Can be EqFilter or null.
	CertificateSha256 *EqFilter `json:"certificate_sha256,omitempty"`
	// Filter by certificate expiration date range. Can be DateRangeFilter or null.
	CertificateExpiresAt *DateRangeFilter `json:"certificate_expires_at,omitempty"`
	// Filter by certificate issuance date range. Can be DateRangeFilter or null.
	CertificateIssuedAt *DateRangeFilter `json:"certificate_issued_at,omitempty"`
	// Filter by certificate issuer common name or organization. Can be EqFilter, InFilter, or null.
	CertificateIssuer interface{} `json:"certificate_issuer,omitempty"` // *EqFilter | *InFilter | nil
	// Filter where cert covers a domain (exact or wildcard). Can be EqFilter, InFilter, ContainsFilter, or null.
	CertificateCoversDomain interface{} `json:"certificate_covers_domain,omitempty"` // *EqFilter | *InFilter | *ContainsFilter | nil
}

// ExposureSeverity represents the severity level of an exposure.
// ENUM(unknown, informational, moderate, critical)
type ExposureSeverity string

const (
	ExposureSeverityUnknown       ExposureSeverity = "unknown"
	ExposureSeverityInformational ExposureSeverity = "informational"
	ExposureSeverityModerate      ExposureSeverity = "moderate"
	ExposureSeverityCritical      ExposureSeverity = "critical"
)

// ExposurePropertiesFilter defines filters based on exposure properties.
type ExposurePropertiesFilter struct {
	// Filter by exposure severity (matches provided level or higher). Can be InFilter or EqFilter.
	Severity interface{} `json:"severity,omitempty"` // *InFilter | *EqFilter (containing ExposureSeverity)
	// Filter by ASI Signature ID (e.g., "cve-2024-6387"). Can be EqFilter or InFilter.
	SignatureID interface{} `json:"signature_id,omitempty"` // *EqFilter | *InFilter
	// Filter by asset exposure score range (0-100).
	AssetExposureScore *IntRangeFilter `json:"asset_exposure_score,omitempty"`
}

// TechnologyPropertiesFilter defines filters based on technology and port properties.
type TechnologyPropertiesFilter struct {
	// Filter by open port number. Can be IntEqFilter, IntInFilter, or null.
	OpenPortNumber interface{} `json:"open_port_number,omitempty"` // *IntEqFilter | *IntInFilter | nil
	// Filter by service on open port (e.g., "http"). Can be EqFilter, InFilter, or null.
	OpenPortService interface{} `json:"open_port_service,omitempty"` // *EqFilter | *InFilter | nil
	// Filter by protocol on open port ("tcp", "udp"). Can be EqFilter, InFilter, or null.
	OpenPortProtocol interface{} `json:"open_port_protocol,omitempty"` // *EqFilter | *InFilter | nil
	// Filter by technology on open port (e.g., "nginx"). Can be EqFilter, InFilter, or null.
	OpenPortTechnology interface{} `json:"open_port_technology,omitempty"` // *EqFilter | *InFilter | nil
	// Filter if a WAF is detected. Can be BooleanFilter or null.
	WAFDetected *BooleanFilter `json:"waf_detected,omitempty"`
	// Filter by specific WAF name (e.g., "Cloudflare"). Can be EqFilter, InFilter, or null.
	WAFName interface{} `json:"waf_name,omitempty"` // *EqFilter | *InFilter | nil
	// Filter by any technology name (port or web). Can be EqFilter, InFilter, or null.
	TechnologyName interface{} `json:"technology_name,omitempty"` // *EqFilter | *InFilter | nil
	// Filter by web technology name (e.g., "jQuery"). Can be EqFilter, InFilter, or null.
	WebTechnologyName interface{} `json:"web_technology_name,omitempty"` // *EqFilter | *InFilter | nil
}

// AssetFilter is the main container for all asset search filters.
type AssetFilter struct {
	// Properties directly tied to the asset or DNS.
	AssetProperties *AssetPropertiesFilter `json:"asset_properties,omitempty"`
	// Properties related to certificates.
	CertificateProperties *CertificatePropertiesFilter `json:"certificate_properties,omitempty"`
	// Properties relating to exposures.
	ExposureProperties *ExposurePropertiesFilter `json:"exposure_properties,omitempty"`
	// Properties related to open ports and technologies.
	TechnologyProperties *TechnologyPropertiesFilter `json:"technology_properties,omitempty"`
}

// Pagination defines pagination parameters for requests.
type Pagination struct {
	// Opaque cursor from a previous response to get the next page.
	NextCursor *string `json:"next_cursor,omitempty"`
	// Number of items to return. Default: 50, Max: 1000.
	Limit int `json:"limit,omitempty" default:"50"`
}

// AssetSearchRequest is the request body for searching assets via POST.
type AssetSearchRequest struct {
	Filter      AssetFilter       `json:"filter"`
	Pagination  *Pagination       `json:"pagination,omitempty"`
	Enrichments []AssetEnrichment `json:"enrichments,omitempty"`
	// Sort order. Can be []AssetSortField or [][2]string{ {field, direction} }.
	Sort interface{} `json:"sort,omitempty"` // []AssetSortField | [][2]string
}

// DNSValue represents a single value within a DNS record set.
type DNSValue struct {
	// The actual DNS record value (e.g., IP address, domain name, MX record details as object).
	Value interface{} `json:"value"`
	// Sources from where this record was seen (optional).
	SeenFrom []string `json:"seen_from,omitempty"`
	// Timestamp when this specific value was first seen (optional). Format: date-time.
	FirstSeenAt *time.Time `json:"first_seen_at,omitempty"`
	// Timestamp when this specific value was last resolved/seen. Format: date-time.
	LastResolvedAt *time.Time `json:"last_resolved_at"` // Required, but schema allows null
}

// DNSRecord represents a DNS record associated with an asset.
type DNSRecord struct {
	RecordType string      `json:"record_type"`          // e.g., "A", "CNAME", "MX"
	Value      *[]DNSValue `json:"value"`                // Required, but schema allows null list
	IsVirtual  bool        `json:"is_virtual,omitempty"` // Is this a virtual DNS record? Default: false.
}

// WHOISContact represents contact information from a WHOIS record.
type WHOISContact struct {
	// Email address of the contact. Format: email.
	Email *string `json:"email,omitempty"`
	// Name of the contact.
	Name *string `json:"name,omitempty"`
	// Organization of the contact.
	Organization *string `json:"organization,omitempty"`
	// Is this still a current contact? Default: true.
	IsCurrent bool `json:"is_current,omitempty"`
}

// WHOISRecord represents WHOIS/RDAP data for a domain asset.
type WHOISRecord struct {
	// Registrar of the domain.
	Registrar *string `json:"registrar,omitempty"`
	// Expiration date of the domain. Format: date-time.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Last updated date of the domain. Format: date-time.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	// Creation date of the domain. Format: date-time.
	CreatedAt *time.Time `json:"created_at,omitempty"`
	// Is the domain registration private?
	IsPrivate *bool `json:"is_private,omitempty"`
	// Is this WHOIS record from the parent domain? Default: false.
	IsFromParent bool `json:"is_from_parent,omitempty"`
	// Contacts associated with the domain.
	Contacts *[]WHOISContact `json:"contacts,omitempty"`
	// Primary nameservers for the domain.
	NameServers *[]string `json:"name_servers,omitempty"`
}

// CertificateEntity represents the subject or issuer of a certificate.
type CertificateEntity struct {
	CommonName             *string `json:"common_name,omitempty"`
	OrganizationName       *string `json:"organization_name,omitempty"`
	OrganizationalUnitName *string `json:"organizational_unit_name,omitempty"`
	CountryName            *string `json:"country_name,omitempty"`
}

// Certificate represents details of an SSL/TLS certificate.
type Certificate struct {
	ExpiresAt       time.Time          `json:"expires_at"` // Format: date-time
	IssuedAt        time.Time          `json:"issued_at"`  // Format: date-time
	Sha256          string             `json:"sha256"`
	Subject         CertificateEntity  `json:"subject"`
	SubjectAltNames *[]string          `json:"subject_alt_names,omitempty"`
	Issuer          *CertificateEntity `json:"issuer,omitempty"`
	// The certificate chain (optional, requested via enrichments).
	Chain *[]Certificate `json:"chain,omitempty"`
	// Signature algorithm used.
	SignatureAlgorithm *string `json:"signature_algorithm,omitempty"`
}

// Port represents the basic details of a port where a certificate was seen.
type Port struct {
	Port     int    `json:"port"`     // The port number
	Protocol string `json:"protocol"` // "tcp" or "udp"
	// Specific instances of this port on IPs (only included in ScannedIP context, not CertificateInstance).
	Instances []PortInstance `json:"instances,omitempty"`
	// Certificate associated with this port (only included in ScannedIP context).
	Certificate *Certificate `json:"certificate,omitempty"`
}

// CertificateInstance links a Certificate to the Ports it was seen on for an asset.
type CertificateInstance struct {
	// The details of the certificate itself.
	Certificate Certificate `json:"certificate"`
	// Where this certificate was seen (ports/protocols). May be null.
	SeenPorts *[]Port `json:"seen_ports,omitempty"` // Note: This Port struct likely won't have Instances or nested Certificate here.
}

// TechnologyInstance represents a specific observation of a technology.
type TechnologyInstance struct {
	// Timestamp when this instance was seen. Format: date-time.
	SeenAt time.Time `json:"seen_at"`
	// Port where this instance was seen.
	SeenPort int `json:"seen_port"`
	// URL where this technology was seen, if applicable. Format: uri.
	SeenURL *string `json:"seen_url,omitempty"`
}

// TechnologyWithInstances represents a detected technology and its instances.
type TechnologyWithInstances struct {
	// Name of the technology (e.g., "Apache httpd", "OpenSSH").
	Name string `json:"name"`
	// Vendor of the product (e.g., "Microsoft", "Apache").
	Vendor *string `json:"vendor,omitempty"`
	// Type of technology (e.g., "web server", "ssh server").
	TechnologyType *string `json:"technology_type,omitempty"`
	// Specific version detected (e.g., "v1.0.0", "v2.3-p1").
	Version *string `json:"version,omitempty"`
	// Specific instances where this technology was observed.
	Instances []TechnologyInstance `json:"instances,omitempty"`
}

// DefensiveControl represents a detected defensive technology.
type DefensiveControl struct {
	// Name of the technology (e.g., "Cloudflare WAF").
	Name string `json:"name"`
	// Vendor of the product (e.g., "Cloudflare").
	Vendor *string `json:"vendor,omitempty"`
	// Type of technology (e.g., "WAF").
	TechnologyType *string `json:"technology_type,omitempty"`
	// Specific version detected.
	Version *string `json:"version,omitempty"`
	// Specific instances where this defense was observed.
	Instances []TechnologyInstance `json:"instances,omitempty"` // Schema reuses TechnologyInstance
}

// ExposureInstance represents a specific instance of an exposure on an asset.
type ExposureInstance struct {
	PortNumber int     `json:"port_number"`
	URL        *string `json:"url,omitempty"` // URL related to the exposure, if applicable
	// Additional details specific to this instance (e.g., version found). May be null.
	Details map[string]interface{} `json:"details,omitempty"`
}

// Exposure represents an identified exposure (vulnerability or misconfiguration).
type Exposure struct {
	ID string `json:"id"` // The ASI Signature ID (e.g., "cve-...")
	// Internal detection identifier. May be null.
	DetectionID *string          `json:"detection_id,omitempty"`
	Severity    ExposureSeverity `json:"severity"`
	// List of specific instances where this exposure was found on the asset.
	Instances []ExposureInstance `json:"instances"`
	// General details about the exposure on this asset (may combine info from instances). May be null.
	Details map[string]interface{} `json:"details,omitempty"`
	// Does this exposure support evidence downloads? May be null.
	SupportsEvidence *bool `json:"supports_evidence,omitempty"`
}

// PortInstance represents details observed on a specific IP and port.
type PortInstance struct {
	// The IP address where this port was seen. Format: ipvanyaddress or ipv6.
	SeenIP string `json:"seen_ip"`
	// Timestamp when this instance was observed. Format: date-time.
	SeenAt time.Time `json:"seen_at"`
	// Service protocol detected (e.g., "http", "rdp").
	Service *string `json:"service,omitempty"`
	// Application running on the port. May be null.
	Technology *TechnologyWithInstances `json:"technology,omitempty"`
	// Web technologies detected on this port. May be null.
	WebTechnologies *[]TechnologyWithInstances `json:"web_technologies,omitempty"`
	// Exposures detected on this port. May be null.
	Exposures *[]Exposure `json:"exposures,omitempty"`
	// Defensive measures detected on this port. May be null.
	Defenses *[]DefensiveControl `json:"defenses,omitempty"`
}

// GeoLocation represents geolocation information.
type GeoLocation struct {
	Continent  *string `json:"continent,omitempty"`   // Continent of the IP address
	Country    *string `json:"country,omitempty"`     // Country of the IP address
	City       *string `json:"city,omitempty"`        // City of the IP address
	CountryISO *string `json:"country_iso,omitempty"` // ISO code of the country
}

// IPMetadata provides ownership and ASN information for an IP address.
type IPMetadata struct {
	ASNumber  *int    `json:"as_number,omitempty"`  // Autonomous System Number
	OwnerName *string `json:"owner_name,omitempty"` // AS or Org Name
	Registry  *string `json:"registry,omitempty"`   // RIR (e.g., "RIPE", "ARIN")
	// Geolocation information for the IP address. May be null.
	OwnerGeo *GeoLocation `json:"owner_geo,omitempty"`
}

// ScannedIP contains details about an IP address associated with an asset that has been scanned.
type ScannedIP struct {
	// The IP address that was scanned. Format: ipvanyaddress.
	IP string `json:"ip"`
	// Timestamp of the last port scan on this IP. Format: date-time. May be null.
	LastScannedAt *time.Time `json:"last_scanned_at,omitempty"`
	// WHOIS data specifically for this IP (if applicable and requested). May be null.
	Whois *WHOISRecord `json:"whois,omitempty"`
	// List of open ports found on this IP. May be null.
	OpenPorts *[]Port `json:"open_ports,omitempty"` // This Port struct includes Instances and Certificate
	// Ownership and ASN information for this IP. May be null.
	Metadata *IPMetadata `json:"metadata,omitempty"`
}

// Asset represents a domain or IP address within a project.
type Asset struct {
	// Project ID this asset belongs to.
	ProjectID string `json:"project_id"`
	// The asset identifier (domain or IP). Same as `name`.
	ID string `json:"id"`
	// The asset identifier (domain or IP). Same as `id`.
	Name string `json:"name"`
	// Type of asset ("domain" or "ip").
	Type string `json:"type"`
	// When ASI first identified this asset. Format: date-time. May be null.
	DiscoveredAt *time.Time `json:"discovered_at"`
	// When this asset was added to the project. Format: date-time.
	AddedToProjectAt time.Time `json:"added_to_project_at"`
	// Last time scanning activity occurred. Format: date-time. May be null.
	LastScannedAt *time.Time `json:"last_scanned_at,omitempty"`
	// Apex domain for `domain` assets (e.g., "example.com"). May be null.
	ApexDomain *string `json:"apex_domain,omitempty"`
	// ASI-calculated exposure score (0-100). May be null.
	ExposureScore *int `json:"exposure_score,omitempty"`
	// Was this asset added manually (`true`) or via rules (`false`). Default: false.
	IsStaticAsset bool `json:"is_static_asset,omitempty"`
	// User-defined tags on this asset. May be null. Returned by default or if requested.
	CustomTags *[]string `json:"custom_tags,omitempty"`
	// IPs the asset resolves to (A/CNAME or self). Format: ipvanyaddress. May be null.
	ResolvedIPs *[]string `json:"resolved_ips,omitempty"`

	// --- Enrichment Fields (Populated based on request parameters) ---

	// DNS records (requires "dns_records" enrichment). May be null.
	DNSRecords *[]DNSRecord `json:"dns_records,omitempty"`
	// WHOIS data (requires "whois" enrichment). May be null.
	Whois *WHOISRecord `json:"whois,omitempty"`
	// Unique certificates found (requires "certificates" enrichment). May be null.
	// Use "certificate_chain" enrichment to populate Certificate.Chain.
	Certificates *[]CertificateInstance `json:"certificates,omitempty"`
	// Defensive measures (requires "defenses" enrichment). May be null.
	Defenses *[]DefensiveControl `json:"defenses,omitempty"`
	// Exposures found (requires "exposures" enrichment). May be null.
	// Use "exposure_instance_details" to populate ExposureInstance.Details.
	Exposures *[]Exposure `json:"exposures,omitempty"`
	// Scanned IP details (requires "ip_metadata", "open_tcp_ports", "open_udp_ports", "web_technologies"). May be null.
	ScannedIPs *[]ScannedIP `json:"scanned_ips,omitempty"`
}

// ApiListResponseAsset is the response structure for listing assets.
type ApiListResponseAsset struct {
	Data []Asset `json:"data"`
	Meta ApiMeta `json:"meta"`
}

// AssetResponse is the response structure for retrieving a single asset.
type AssetResponse struct {
	Data Asset   `json:"data"`
	Meta ApiMeta `json:"meta"`
}

// ValidationError details a specific validation error.
type ValidationError struct {
	// Location of the error (e.g., ["query", "limit"]).
	Loc  []interface{} `json:"loc"`  // Can contain string or int
	Msg  string        `json:"msg"`  // Error message
	Type string        `json:"type"` // Type of error
}

// HTTPValidationError represents a response for validation errors (HTTP 422).
type HTTPValidationError struct {
	Detail []ValidationError `json:"detail,omitempty"`
}

// VulnerabilityPublic represents public details about a CVE or vulnerability.
type VulnerabilityPublic struct {
	Name string `json:"name"`
	// CVE identifier (e.g., "CVE-2024-6387"). May be null.
	CveID *string `json:"cve_id,omitempty"`
	// Unique slug identifier for the vulnerability.
	Slug string `json:"slug"`
	// Associated CWE IDs. Items can be string or null.
	CweIDs []string `json:"cwe_ids,omitempty"` // Schema allows nulls in array
	// CVSS score. May be null.
	CvssScore *float64 `json:"cvss_score"` // Required by schema, but allows null
	// CVSS metrics string. May be null.
	CvssMetrics *string `json:"cvss_metrics"` // Required by schema, but allows null
	// EPSS score. May be null.
	EpssScore *float64 `json:"epss_score,omitempty"`
	// List of reference URLs.
	References []string `json:"references"`
}

// ExposureSignatureResponse contains details about an exposure signature.
type ExposureSignatureResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Description of the exposure. May be null.
	Description *string `json:"description"` // Required by schema, but allows null
	// Severity of the exposure. May be null.
	Severity *ExposureSeverity `json:"severity"` // Required by schema, but allows null
	// Suggested remediation steps (structure varies). May be null.
	RemediationSteps map[string]interface{} `json:"remediation_steps,omitempty"`
	// Date the signature was added to ASI. Format: date-time. May be null.
	AddedAt *time.Time `json:"added_at,omitempty"`
	// List of reference URLs. May be null.
	References *[]string `json:"references"` // Required by schema, but allows null
	// Associated vulnerabilities (e.g., CVEs). May be null.
	Vulnerabilities *[]VulnerabilityPublic `json:"vulnerabilities,omitempty"`
}

// AssetExposure bundles exposure signature details with instances on a specific asset.
type AssetExposure struct {
	AssetID string `json:"asset_id"`
	// Specific instances of this exposure on the asset.
	Instances []ExposureInstance `json:"instances"`
	// Aggregated details for this exposure on the asset. May be null.
	Details   map[string]interface{}    `json:"details"` // Required by schema, but allows null
	Signature ExposureSignatureResponse `json:"signature"`
}

// ApiListResponseAssetExposure is the response for listing detailed asset exposures.
type ApiListResponseAssetExposure struct {
	Data []AssetExposure `json:"data"`
	Meta ApiMeta         `json:"meta"`
}

// NameValuePair is a simple structure for name-value pairs used in filters.
type NameValuePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AssetCountFilter is used within filter options, showing asset counts per value.
type AssetCountFilter struct {
	Name       string `json:"name"`        // Display name for the filter value
	Value      string `json:"value"`       // The actual value to use in a query
	AssetCount int    `json:"asset_count"` // Number of assets matching this filter value
}

// FilterOption represents a filterable field and its available values/counts.
type FilterOption struct {
	Name        string             `json:"name"`         // Display name of the filter field (e.g., "ASN")
	FilterQuery string             `json:"filter_query"` // Query parameter name for GET requests (e.g., "asn")
	FilterPath  string             `json:"filter_path"`  // Dotted path for POST search requests (e.g., "asset_properties.asn")
	Filters     []AssetCountFilter `json:"filters"`      // Available values and counts for this filter
}

// AssetPropertiesFilterOptions holds available filter options related to asset properties.
type AssetPropertiesFilterOptions struct {
	// Asset type filter options (usually static: "ip", "domain"). Default provided.
	AssetType []NameValuePair `json:"asset_type,omitempty"`
	// Available ASNs and their counts.
	ASN []NameValuePair `json:"asn,omitempty"` // Schema inconsistency: Should likely be FilterOption or AssetCountFilter list
	// Available certificate issuers and counts.
	CertificateIssuer *FilterOption `json:"certificate_issuer,omitempty"`
	// Available custom tags and counts.
	CustomTags *FilterOption `json:"custom_tags,omitempty"`
}

// ExposurePropertiesFilterOptions holds available filter options related to exposure properties.
type ExposurePropertiesFilterOptions struct {
	// Available exposure signature IDs and their counts.
	SignatureID []AssetCountFilter `json:"signature_id,omitempty"` // Schema shows AssetCountFilter here
}

// FiltersResponse is the response structure for the GET /filters endpoint.
type FiltersResponse struct {
	AssetProperties    AssetPropertiesFilterOptions    `json:"asset_properties"`
	ExposureProperties ExposurePropertiesFilterOptions `json:"exposure_properties"`
}

// AssetTagResponse is the data part of the response for tagging operations.
type AssetTagResponse struct {
	// Tags requested to be added. May be null.
	AddTags *[]string `json:"add_tags,omitempty"`
	// Tags requested to be removed. May be null.
	RemoveTags *[]string `json:"remove_tags,omitempty"`
	// Assets involved in the tagging action. May be null.
	Assets *[]string `json:"assets,omitempty"`
	// Whether the backend task submission is complete. Default: false.
	// NOTE: Indexing might still take time even if true.
	Complete bool `json:"complete,omitempty"`
	// Task IDs to check status if Complete is false. May be null.
	TaskIDs *[]string `json:"task_ids,omitempty"`
}

// AssetTagAPIResponse is the full API response for tagging operations.
type AssetTagAPIResponse struct {
	Data AssetTagResponse `json:"data"`
}

// TagAssetRequest defines tags to add or remove for a single asset.
type TagAssetRequest struct {
	// List of tag names to apply. May be null.
	AddTags *[]string `json:"add_tags,omitempty"`
	// List of tag names to remove. May be null.
	RemoveTags *[]string `json:"remove_tags,omitempty"`
}

// CustomTagPublic represents a user-defined tag.
type CustomTagPublic struct {
	Title string `json:"title"` // The name of the tag
}

// ApiListResponseCustomTagPublic is the response for listing custom tags.
type ApiListResponseCustomTagPublic struct {
	Data []CustomTagPublic `json:"data"`
	Meta ApiMeta           `json:"meta"`
}

// BulkTagAssetsRequest is the request body for bulk tagging assets.
type BulkTagAssetsRequest struct {
	// Map where keys are asset IDs (IP/domain) and values are TagAssetRequest objects.
	AssetTags map[string]TagAssetRequest `json:"asset_tags"`
}

// ExposureSummary provides a summary of an exposure and the count of affected assets.
type ExposureSummary struct {
	Signature  ExposureSignatureResponse `json:"signature"`
	AssetCount int                       `json:"asset_count"` // Number of assets affected by this exposure
}

// ApiListResponseExposureSummary is the response for listing exposure summaries.
type ApiListResponseExposureSummary struct {
	Data []ExposureSummary `json:"data"`
	Meta ApiMeta           `json:"meta"`
}

// AssetWithExposureInstances pairs an asset ID with its specific exposure instances for a given signature.
type AssetWithExposureInstances struct {
	AssetID string `json:"asset_id"`
	// Instances of the specific exposure on this asset.
	Instances []ExposureInstance `json:"instances"`
	// Aggregated details relevant to this exposure on this asset. May be null.
	Details map[string]interface{} `json:"details"` // Required by schema, but allows null
}

// ExposureAssets contains the signature details and the list of affected assets with their instances.
type ExposureAssets struct {
	Signature      ExposureSignatureResponse    `json:"signature"`
	AssetExposures []AssetWithExposureInstances `json:"asset_exposures"`
}

// ExposureAssetsListResponse is the response structure for getting assets affected by a specific exposure.
type ExposureAssetsListResponse struct {
	Data ExposureAssets `json:"data"`
	Meta ApiMeta        `json:"meta"`
}
