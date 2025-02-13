package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"manifold/internal/documents"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type EmbeddingRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	EncodingFormat string   `json:"encoding_format"`
}

type EmbeddingResponse struct {
	Object string       `json:"object"`
	Data   []Embedding  `json:"data"`
	Model  string       `json:"model"`
	Usage  UsageMetrics `json:"usage"`
}

type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type UsageMetrics struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Connect takes a connection string and returns a connection to the database
func Connect(ctx context.Context, connStr string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// ProcessDocument processes a document by splitting it into chunks,
// prepending the file path header to each chunk, and saving it to the database.
func ProcessDocument(ctx context.Context, conn *pgx.Conn, embeddingsHost string, apiKey string, doc string, language string, chunkSize int, chunkOverlap int, filePath string) error {
	chunks, err := SplitDocument(doc, language, chunkSize, chunkOverlap)
	if err != nil {
		return err
	}

	log.Printf("Split document into %d chunks", len(chunks))

	// Prepend the file path header to each chunk if provided.
	if filePath != "" {
		for i, chunk := range chunks {
			chunks[i] = fmt.Sprintf("[%s] %s", filePath, chunk)
		}
	}

	embeddings, err := GenerateEmbeddings(embeddingsHost, apiKey, chunks)
	if err != nil {
		return err
	}

	err = SaveDocument(ctx, conn, embeddingsHost, apiKey, chunks, embeddings)
	if err != nil {
		return err
	}

	return nil
}

// RetrieveDocuments retrieves the most similar documents from the database
func RetrieveDocuments(ctx context.Context, conn *pgx.Conn, embeddingsHost string, apiKey string, content string, limit int) (string, error) {
	// Retrieve the most similar document to the content
	promptEmbeddings, err := GenerateEmbeddings(embeddingsHost, apiKey, []string{content})
	if err != nil {
		return "", err
	}

	// Get the nearest neighbors to a vector
	rows, err := conn.Query(ctx, "SELECT id, content, embedding FROM documents ORDER BY embedding <-> $1 LIMIT $2", pgvector.NewVector(promptEmbeddings[0]), limit)
	if err != nil {
		return "", err
	}

	defer rows.Close()

	documents := make([]string, 0)

	for rows.Next() {
		var id int64
		var content string
		var embedding pgvector.Vector
		err = rows.Scan(&id, &content, &embedding)
		if err != nil {
			return "", err
		}

		// Create a delimiter for the document
		documents = append(documents, "### Document ###")
		documents = append(documents, content)

		// Append double new lines to separate the documents
		documents = append(documents, "\n\n")

		log.Println(id, content, embedding)
	}

	// Convert the documents to a single string
	documentsString := strings.Join(documents, "### Document ###")

	return documentsString, nil
}

// SplitDocument splits a document into chunks of text
func SplitDocument(doc string, language string, chunkSize int, chunkOverlap int) ([]string, error) {
	splitter, err := documents.FromLanguage(documents.Language(language))
	if err != nil {
		return nil, err
	}

	chunks := splitter.SplitText(doc)
	return chunks, nil
}

// SaveDocument saves a document and its vector embeddings to the pgvector database
func SaveDocument(ctx context.Context, conn *pgx.Conn, embeddingsHost string, apiKey string, chunks []string, embeddings [][]float32) error {
	for i, content := range chunks {
		_, err := conn.Exec(ctx, "INSERT INTO documents (content, embedding) VALUES ($1, $2)", content, pgvector.NewVector(embeddings[i]))
		if err != nil {
			return err
		}
	}
	return nil
}

// GenerateEmbeddings generates embeddings for a given text
func GenerateEmbeddings(host string, apiKey string, chunks []string) ([][]float32, error) {
	// Create a new EmbeddingRequest
	embeddingRequest := EmbeddingRequest{
		Input:          chunks,
		Model:          "nomic-embed-text-v1.5.Q8_0",
		EncodingFormat: "float",
	}

	embeddings, err := FetchEmbeddings(host, embeddingRequest, apiKey)
	if err != nil {
		panic(err)
	}

	return embeddings, nil
}

func FetchEmbeddings(host string, request EmbeddingRequest, apiKey string) ([][]float32, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	// Print the request for debugging purposes.
	fmt.Println(string(b))

	req, err := http.NewRequest("POST", host, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	var embeddings [][]float32
	for _, item := range result["data"].([]interface{}) {
		var embedding []float32
		for _, v := range item.(map[string]interface{})["embedding"].([]interface{}) {
			embedding = append(embedding, float32(v.(float64)))
		}
		embeddings = append(embeddings, embedding)
	}
	return embeddings, nil
}
