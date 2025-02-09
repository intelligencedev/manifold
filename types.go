package main

// RepoConcatRequest represents the request body for the /api/repoconcat endpoint.
type RepoConcatRequest struct {
	Paths         []string `json:"paths"`
	Types         []string `json:"types"`
	Recursive     bool     `json:"recursive"`
	IgnorePattern string   `json:"ignorePattern"`
}

// DatadogNodeRequest represents the structure of the incoming request from the frontend.
type DatadogNodeRequest struct {
	APIKey    string `json:"apiKey"`
	AppKey    string `json:"appKey"`
	Site      string `json:"site"`
	Operation string `json:"operation"`
	Query     string `json:"query"`
	FromTime  string `json:"fromTime"`
	ToTime    string `json:"toTime"`
}

// DatadogNodeResponse represents the structure of the response sent back to the frontend.
type DatadogNodeResponse struct {
	Result struct {
		Output interface{} `json:"output"`
	} `json:"result"`
}
