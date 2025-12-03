package databases

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

// Qdrant only allows UUIDs and positive integers as point IDs.
// So we generate a deterministic UUID based on the original ID.
// And store the original ID in the payload.
const PAYLOAD_ID_FIELD = "_original_id"

type qdrantVector struct {
	client     *qdrant.Client
	collection string
	dimension  int
	metric     string // cosine|l2|euclidean|ip|dot|manhattan
}

// Creates a new Qdrant vector store.
// Note: The Go client uses Qdrant's gRPC API, which runs on port 6334 by default.
//
// Optionally, an API key can be provided as a query parameter: "http://localhost:6334?api_key=your_api_key"
func NewQdrantVector(dsn string, collection string, dimensions int, metric string) (VectorStore, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection name is required")
	}
	parsedURL, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse Qdrant DSN: %w", err)
	}
	host := parsedURL.Hostname()
	if host == "" {
		host = "localhost"
	}
	port := parsedURL.Port()
	if port == "" {
		port = "6334"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("invalid port in Qdrant DSN: %w", err)
	}
	config := &qdrant.Config{
		Host: host,
		Port: portNum,
	}
	if parsedURL.Scheme == "https" {
		config.UseTLS = true
	}

	if apiKey := parsedURL.Query().Get("api_key"); apiKey != "" {
		config.APIKey = apiKey
	}
	client, err := qdrant.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("create Qdrant client: %w", err)
	}
	qv := &qdrantVector{
		client:     client,
		collection: collection,
		dimension:  dimensions,
		metric:     strings.ToLower(strings.TrimSpace(metric)),
	}
	ctx := context.Background()
	if err := qv.ensureCollection(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("ensure collection: %w", err)
	}
	return qv, nil
}

func (q *qdrantVector) ensureCollection(ctx context.Context) error {
	exists, err := q.client.CollectionExists(ctx, q.collection)
	if err != nil {
		return fmt.Errorf("check collection exists: %w", err)
	}
	if exists {
		return nil
	}
	var distance qdrant.Distance
	switch q.metric {
	case "l2", "euclidean":
		distance = qdrant.Distance_Euclid
	case "ip", "dot":
		distance = qdrant.Distance_Dot
	case "manhattan":
		distance = qdrant.Distance_Manhattan
	default: // cosine
		distance = qdrant.Distance_Cosine
	}
	vecSize := uint64(q.dimension)
	if q.dimension <= 0 {
		return fmt.Errorf("qdrant requires dimensions > 0")
	}
	err = q.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: q.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     vecSize,
			Distance: distance,
		}),
	})
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

func (q *qdrantVector) Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	uuidStr := id
	if _, err := uuid.Parse(id); err != nil {
		uuidStr = uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String()
	}
	metadataAny := make(map[string]any, len(metadata)+1)
	for k, v := range metadata {
		metadataAny[k] = v
	}
	if uuidStr != id {
		metadataAny[PAYLOAD_ID_FIELD] = id
	}
	payload := qdrant.NewValueMap(metadataAny)
	pointID := qdrant.NewIDUUID(uuidStr)
	vec := make([]float32, len(vector))
	copy(vec, vector)
	points := []*qdrant.PointStruct{
		{
			Id:      pointID,
			Vectors: qdrant.NewVectorsDense(vec),
			Payload: payload,
		},
	}
	_, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: q.collection,
		Points:         points,
	})
	return err
}

func (q *qdrantVector) Delete(ctx context.Context, id string) error {
	uuidStr := id
	if _, err := uuid.Parse(id); err != nil {
		uuidStr = uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String()
	}
	pointID := qdrant.NewIDUUID(uuidStr)
	_, err := q.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: q.collection,
		Points:         qdrant.NewPointsSelector(pointID),
	})
	return err
}

func (q *qdrantVector) SimilaritySearch(ctx context.Context, vector []float32, k int, filter map[string]string) ([]VectorResult, error) {
	if k <= 0 {
		k = 10
	}
	vec := make([]float32, len(vector))
	copy(vec, vector)
	var queryFilter *qdrant.Filter
	if len(filter) > 0 {
		must := make([]*qdrant.Condition, 0, len(filter))
		for k, v := range filter {
			must = append(must, qdrant.NewMatch(k, v))
		}
		queryFilter = &qdrant.Filter{
			Must: must,
		}
	}
	limit := uint64(k)
	searchResult, err := q.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: q.collection,
		Query:          qdrant.NewQueryDense(vec),
		Limit:          &limit,
		Filter:         queryFilter,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}
	results := make([]VectorResult, 0, len(searchResult))
	for _, hit := range searchResult {
		uuidStr := hit.Id.GetUuid()
		if uuidStr == "" {
			uuidStr = hit.Id.String()
		}
		metadata := make(map[string]string)
		var originalID string
		if hit.Payload != nil {
			for k, v := range hit.Payload {
				if k == PAYLOAD_ID_FIELD {
					originalID = v.GetStringValue()
					continue
				}
				metadata[k] = v.GetStringValue()
			}
		}
		id := originalID
		if id == "" {
			id = uuidStr
		}
		score := float64(hit.Score)
		results = append(results, VectorResult{
			ID:       id,
			Score:    score,
			Metadata: metadata,
		})
	}
	return results, nil
}

func (q *qdrantVector) Dimension() int { return q.dimension }

func (q *qdrantVector) Close() error {
	return q.client.Close()
}
