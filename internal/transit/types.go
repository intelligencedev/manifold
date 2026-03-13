package transit

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	defaultSearchLimit = 10
	defaultListLimit   = 100
	defaultBatchSize   = 100
	maxKeyLength       = 512
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_./@-]+$`)

type Record struct {
	ID          string    `json:"id"`
	TenantID    int64     `json:"tenantId"`
	KeyName     string    `json:"keyName"`
	Description string    `json:"description"`
	Value       string    `json:"value"`
	Base64      bool      `json:"base64"`
	Embed       bool      `json:"embed"`
	EmbedSource string    `json:"embedSource"`
	Version     int64     `json:"version"`
	CreatedBy   int64     `json:"createdBy"`
	UpdatedBy   int64     `json:"updatedBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Metadata struct {
	KeyName     string    `json:"keyName"`
	Description string    `json:"description"`
	Base64      bool      `json:"base64"`
	Embed       bool      `json:"embed"`
	EmbedSource string    `json:"embedSource"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateMemoryItem struct {
	KeyName     string `json:"keyName"`
	Description string `json:"description"`
	Value       string `json:"value"`
	Base64      *bool  `json:"base64,omitempty"`
	Embed       *bool  `json:"embed,omitempty"`
	EmbedSource string `json:"embedSource,omitempty"`
}

type UpdateMemoryRequest struct {
	KeyName     string `json:"keyName"`
	Value       string `json:"value"`
	Base64      *bool  `json:"base64,omitempty"`
	Embed       *bool  `json:"embed,omitempty"`
	EmbedSource string `json:"embedSource,omitempty"`
	IfVersion   int64  `json:"ifVersion,omitempty"`
}

type SearchRequest struct {
	Query         string     `json:"query"`
	Prefix        string     `json:"prefix,omitempty"`
	Limit         int        `json:"limit,omitempty"`
	WithinDays    int        `json:"withinDays,omitempty"`
	CreatedAfter  *time.Time `json:"createdAfter,omitempty"`
	CreatedBefore *time.Time `json:"createdBefore,omitempty"`
	UpdatedAfter  *time.Time `json:"updatedAfter,omitempty"`
	UpdatedBefore *time.Time `json:"updatedBefore,omitempty"`
}

type ListRequest struct {
	Prefix string `json:"prefix,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type SearchHit struct {
	Record  Record  `json:"record"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet,omitempty"`
}

type DiscoverHit struct {
	Metadata Metadata `json:"metadata"`
	Score    float64  `json:"score"`
	Snippet  string   `json:"snippet,omitempty"`
}

type SearchCandidate struct {
	Record  Record
	Score   float64
	Snippet string
}

func ValidateKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("keyName is required")
	}
	if len(key) > maxKeyLength {
		return fmt.Errorf("keyName exceeds %d characters", maxKeyLength)
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("keyName cannot contain '..'")
	}
	if strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/") {
		return fmt.Errorf("keyName cannot start or end with '/'")
	}
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("keyName contains unsupported characters")
	}
	return nil
}

func NormalizeEmbedSource(src string) string {
	src = strings.ToLower(strings.TrimSpace(src))
	if src == "description" {
		return "description"
	}
	return "value"
}

func ApplyCreateDefaults(item CreateMemoryItem) CreateMemoryItem {
	if item.Base64 == nil {
		value := false
		item.Base64 = &value
	}
	if item.Embed == nil {
		value := true
		item.Embed = &value
	}
	item.EmbedSource = NormalizeEmbedSource(item.EmbedSource)
	return item
}

func MetadataFromRecord(record Record) Metadata {
	return Metadata{
		KeyName:     record.KeyName,
		Description: record.Description,
		Base64:      record.Base64,
		Embed:       record.Embed,
		EmbedSource: record.EmbedSource,
		Version:     record.Version,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}
