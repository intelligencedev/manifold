package magma

import (
	"strings"
	"time"
)

type GraphType string

const (
	GraphSemantic GraphType = "semantic"
	GraphTemporal GraphType = "temporal"
	GraphCausal   GraphType = "causal"
	GraphEntity   GraphType = "entity"
)

type IntentCategory uint8

const (
	IntentTemporal IntentCategory = 1 << iota
	IntentEntity
	IntentSemantic
	IntentCausal
)

func (i IntentCategory) String() string {
	if i == 0 {
		return "auto"
	}
	parts := make([]string, 0, 4)
	if i&IntentTemporal != 0 {
		parts = append(parts, "temporal")
	}
	if i&IntentEntity != 0 {
		parts = append(parts, "entity")
	}
	if i&IntentSemantic != 0 {
		parts = append(parts, "semantic")
	}
	if i&IntentCausal != 0 {
		parts = append(parts, "causal")
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, "+")
}

type TemporalAttrs struct {
	Date       string `json:"date,omitempty"`
	RelativeTo string `json:"relative_to,omitempty"`
	Offset     string `json:"offset,omitempty"`
}

type EntityMention struct {
	ID   string `json:"id"`
	Type string `json:"type,omitempty"`
	Role string `json:"role,omitempty"`
	Name string `json:"name,omitempty"`
}

type EventNode struct {
	ID             string          `json:"id"`
	Tenant         string          `json:"tenant,omitempty"`
	Session        string          `json:"session,omitempty"`
	Text           string          `json:"text"`
	Embedding      []float32       `json:"-"`
	Graphs         []GraphType     `json:"graphs,omitempty"`
	SemanticTopK   int             `json:"semantic_top_k,omitempty"`
	TemporalAttrs  TemporalAttrs   `json:"temporal_attrs,omitempty"`
	EntityMentions []EntityMention `json:"entity_mentions,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type Edge struct {
	Source    string
	GraphType GraphType
	Rel       string
	Target    string
	Weight    float64
	Props     map[string]any
}

type BatchUpsertRequest struct {
	Event         EventNode
	TemporalAttrs TemporalAttrs
	Entities      []EntityMention
	Edges         []Edge
}

type AnchorType string

const (
	AnchorVector  AnchorType = "vector"
	AnchorKeyword AnchorType = "keyword"
	AnchorEntity  AnchorType = "entity"
)

type TraversalPolicy struct {
	Intent         IntentCategory
	GraphViews     []GraphType
	MaxHops        int
	MaxNodes       int
	AnchorStrategy AnchorType
}

type QueryOptions struct {
	Tenant               string
	IntentHint           IntentCategory
	MaxHops              int
	MaxNodes             int
	ContextFormat        string
	IntentClassification string
}

type Subgraph struct {
	GraphType GraphType
	Nodes     map[string]EventNode
	Edges     []Edge
}

type TimelineEntry struct {
	EventID string
	Date    string
	Text    string
}

type EntityProfile struct {
	Entity EntityMention
	Events []EventNode
}

type CausalLink struct {
	Cause  string
	Effect string
	Text   string
}

type SemanticGroup struct {
	Topic  string
	Events []EventNode
}

type StructuredContext struct {
	TemporalTimeline []TimelineEntry
	EntityProfile    map[string]EntityProfile
	CausalChain      []CausalLink
	SemanticCluster  []SemanticGroup
	RawEvents        []EventNode
	Text             string
	Intent           IntentCategory
	GraphViews       []GraphType
	AnchorStrategy   AnchorType
	MaxHops          int
	MaxNodes         int
	AnchorCount      int
}

type IngestRequest struct {
	ID           string
	Tenant       string
	SessionID    string
	Text         string
	Metadata     map[string]any
	Graphs       []GraphType
	SemanticTopK int
	CreatedAt    time.Time
}

type EventIngestResponse struct {
	EventID string
	Status  string
}

type ServiceStats struct {
	QueueDepth     int
	ProcessedTotal uint64
	FailedTotal    uint64
	DroppedTotal   uint64
	LastError      string
}

type ServiceConfig struct {
	QueueSize           int
	SemanticTopK        int
	SimilarityThreshold float64
	Graphs              GraphConfig
	Observer            Observer
}

type GraphConfig struct {
	Semantic bool
	Temporal bool
	Causal   bool
	Entity   bool
}

type Observer interface {
	IncCounter(name string, labels map[string]string)
	ObserveHistogram(name string, value float64, labels map[string]string)
}
