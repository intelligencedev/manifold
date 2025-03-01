package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
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

type SummarizeOutput struct {
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords,omitempty"`
}

// Connect takes a connection string and returns a connection to the database
func Connect(ctx context.Context, connStr string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// GenerateEmbeddings generates embeddings for a given text
func GenerateEmbeddings(host string, apiKey string, chunks []string) ([][]float32, error) {
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

// summarizeContent sends the file content to the /v1/chat/completions endpoint to obtain a summary.
func summarizeContent(ctx context.Context, content string, endpoint string, apiKey string) (SummarizeOutput, error) {
	summaryInstructions := `You are an expert code summarizer designed to create concise and informative summaries of code snippets for use in a Retrieval-Augmented Generation (RAG) system. Your goal is to generate summaries that maximize the effectiveness of the RAG system by enabling it to retrieve the most relevant code snippets based on user queries about code functionality and behavior.

**Instructions:**

1. Carefully analyze the following code snippet and its surrounding context (if any is provided). Understand the purpose of the code, its inputs, outputs, and any potential side effects.
2. Generate a short, self-contained summary (2-3 sentences maximum) that describes the core functionality of the code snippet.
3. Focus on creating a summary that answers potential user questions about the code's purpose, usage, and relationship to other parts of the codebase.
4. Prioritize information that is likely to be useful to a developer searching for code that performs a specific task or solves a particular problem.
5. Include relevant keywords related to the code's functionality (e.g., "data validation," "API call," "file parsing," "sorting algorithm") and any key data structures or libraries used.
6. If applicable, mention the programming language, important function names, and relevant classes or modules.
7. Avoid overly technical jargon or implementation details. Summarize the *what* and *why* of the code, rather than the *how* (unless the *how* is crucial for understanding the functionality).
8. Maintain the original code's level of abstraction. Don't oversimplify or overcomplicate the summary.
9. Do not include information that is not directly derived from the code snippet. Avoid speculation or inference about the code's broader context.
10. The output should be a single paragraph.
	`

	reqPayload := map[string]interface{}{
		"model": "local",
		"messages": []map[string]string{
			{"role": "system", "content": summaryInstructions},
			{"role": "user", "content": "Please summarize:\n" + content},
		},
		"max_completion_tokens": 2048,
		"temperature":           0.6,
		"stream":                false,
	}
	reqBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return SummarizeOutput{}, err
	}

	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "http://" + endpoint
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(reqBytes))
	if err != nil {
		return SummarizeOutput{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return SummarizeOutput{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return SummarizeOutput{}, fmt.Errorf("failed to summarize content, status: %d, body: %s", resp.StatusCode, body)
	}

	var respData struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return SummarizeOutput{}, err
	}
	if len(respData.Choices) == 0 {
		return SummarizeOutput{}, fmt.Errorf("no completion choices returned")
	}

	summaryText := respData.Choices[0].Message.Content
	log.Printf("Summary: %s", summaryText)

	// Call the keyword extraction function to retrieve a comma-delimited list of keywords.
	keywords, err := extractKeywords(ctx, summaryText, endpoint, apiKey)
	if err != nil {
		return SummarizeOutput{}, err
	}

	return SummarizeOutput{
		Summary:  summaryText,
		Keywords: keywords,
	}, nil
}

// extractKeywords calls the LLM with a tuned system prompt to extract keywords.
// The LLM should return a comma delimited list of keywords which we then parse.
func extractKeywords(ctx context.Context, summary string, endpoint string, apiKey string) ([]string, error) {
	keywordInstructions := `You are a specialized keyword extractor. Given the summary text of a code snippet, extract the most relevant keywords that represent the core concepts and functionality. Return the keywords as a comma-delimited list with no additional text.`

	reqPayload := map[string]interface{}{
		"model": "local",
		"messages": []map[string]string{
			{"role": "system", "content": keywordInstructions},
			{"role": "user", "content": "Please extract keywords from the following summary:\n" + summary},
		},
		"max_completion_tokens": 256,
		"temperature":           0.6,
		"stream":                false,
	}
	reqBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "http://" + endpoint
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to extract keywords, status: %d, body: %s", resp.StatusCode, body)
	}

	var respData struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, err
	}
	if len(respData.Choices) == 0 {
		return nil, fmt.Errorf("no keyword extraction choices returned")
	}

	keywordsText := respData.Choices[0].Message.Content
	// Parse the comma-delimited list of keywords.
	parts := strings.Split(keywordsText, ",")
	var keywords []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			keywords = append(keywords, trimmed)
		}
	}
	return keywords, nil
}
