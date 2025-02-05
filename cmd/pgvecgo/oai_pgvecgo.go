// Example from: https://github.com/pgvector/pgvector-go/blob/master/examples/openai/main.go

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
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

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("Set OPENAI_API_KEY")
		os.Exit(1)
	}

	// get env vars for user and password
	user := os.Getenv("PGVECTOR_USER")
	if user == "" {
		fmt.Println("Set PGVECTOR_USER")
		os.Exit(1)
	}

	password := os.Getenv("PGVECTOR_PASSWORD")
	if password == "" {
		fmt.Println("Set PGVECTOR_PASSWORD")
		os.Exit(1)
	}

	// get env var for host and database name
	host := os.Getenv("PGVECTOR_HOST")
	if host == "" {
		fmt.Println("Set PGVECTOR_HOST")
		os.Exit(1)
	}

	dbName := os.Getenv("PGVECTOR_DBNAME")
	if dbName == "" {
		fmt.Println("Set PGVECTOR_DBNAME")
		os.Exit(1)
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, password, host, dbName)

	// get env var for embeddings host
	embeddingsHost := os.Getenv("EMBEDDINGS_HOST")
	if embeddingsHost == "" {
		fmt.Println("Set EMBEDDINGS_HOST")
		os.Exit(1)
	}

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		panic(err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		panic(err)
	}

	err = pgxvector.RegisterTypes(ctx, conn)
	if err != nil {
		panic(err)
	}

	_, err = conn.Exec(ctx, "DROP TABLE IF EXISTS documents")
	if err != nil {
		panic(err)
	}

	_, err = conn.Exec(ctx, "CREATE TABLE documents (id bigserial PRIMARY KEY, content text, embedding vector(768))")
	if err != nil {
		panic(err)
	}

	input := []string{
		"The dog is barking",
		"The cat is purring",
		"The bear is growling",
	}

	// Create a new EmbeddingRequest
	embeddingRequest := EmbeddingRequest{
		Input:          input,
		Model:          "nomic-embed-text-v1.5.Q8_0",
		EncodingFormat: "float",
	}

	embeddings, err := FetchEmbeddings(embeddingsHost, embeddingRequest, apiKey)
	if err != nil {
		panic(err)
	}

	for i, content := range input {
		_, err := conn.Exec(ctx, "INSERT INTO documents (content, embedding) VALUES ($1, $2)", content, pgvector.NewVector(embeddings[i]))
		if err != nil {
			panic(err)
		}
	}

	documentId := 1
	rows, err := conn.Query(ctx, "SELECT id, content, embedding FROM documents WHERE id != $1 ORDER BY embedding <=> (SELECT embedding FROM documents WHERE id = $1) LIMIT 5", documentId)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var content string
		var embedding pgvector.Vector
		err = rows.Scan(&id, &content, &embedding)
		if err != nil {
			panic(err)
		}
		fmt.Println(id, content, embedding)
	}

	if rows.Err() != nil {
		panic(rows.Err())
	}
}

type apiRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

func FetchEmbeddings(host string, request EmbeddingRequest, apiKey string) ([][]float32, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	// print the request
	fmt.Println(string(b))

	req, err := http.NewRequest("POST", host, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}

	//req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bad status code: %d", resp.StatusCode)
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
