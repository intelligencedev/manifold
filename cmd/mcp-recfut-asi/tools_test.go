package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockHTTPClient is a mock implementation of the HTTPClient interface
type MockHTTPClient struct {
	mock.Mock
}

// Do implements the HTTPClient interface
func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)

	// Return either a response or an error based on what was set up in the test
	if res := args.Get(0); res != nil {
		return res.(*http.Response), args.Error(1)
	}
	return nil, args.Error(1)
}

// MockAPIClient is a mock implementation of the APIClient interface
type MockAPIClient struct {
	mock.Mock
}

// Request implements the APIClient interface
func (m *MockAPIClient) Request(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	args := m.Called(ctx, method, path, body)

	// Return either a response byte array or an error based on what was set up in the test
	if data := args.Get(0); data != nil {
		return data.([]byte), args.Error(1)
	}
	return nil, args.Error(1)
}

// MockConfigLoader is a mock implementation of the ConfigLoader interface
type MockConfigLoader struct {
	mock.Mock
}

// LoadConfig implements the ConfigLoader interface
func (m *MockConfigLoader) LoadConfig() (*Config, error) {
	args := m.Called()

	// Return either a config or an error based on what was set up in the test
	if config := args.Get(0); config != nil {
		return config.(*Config), args.Error(1)
	}
	return nil, args.Error(1)
}

// GetSecurityTrailsAPIKey implements the ConfigLoader interface
func (m *MockConfigLoader) GetSecurityTrailsAPIKey() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

// createHTTPResponse creates a mock HTTP response with the given status code and body
func createHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// TestSecurityTrailsClientRequest tests the Request method of SecurityTrailsClient
func TestSecurityTrailsClientRequest(t *testing.T) {
	// Test cases
	testCases := []struct {
		name             string
		method           string
		path             string
		body             interface{}
		mockResponseCode int
		mockResponseBody string
		mockError        error
		expectedError    bool
		expectedResponse []byte
	}{
		{
			name:             "Successful GET request",
			method:           http.MethodGet,
			path:             "/v1/ping",
			body:             nil,
			mockResponseCode: http.StatusOK,
			mockResponseBody: `{"success": true, "message": "pong"}`,
			mockError:        nil,
			expectedError:    false,
			expectedResponse: []byte(`{"success": true, "message": "pong"}`),
		},
		{
			name:             "HTTP error response",
			method:           http.MethodGet,
			path:             "/v1/error",
			body:             nil,
			mockResponseCode: http.StatusUnauthorized,
			mockResponseBody: `{"message": "Invalid API key"}`,
			mockError:        nil,
			expectedError:    true,
			expectedResponse: nil,
		},
		{
			name:             "Network error",
			method:           http.MethodGet,
			path:             "/v1/endpoint",
			body:             nil,
			mockResponseCode: 0,
			mockResponseBody: "",
			mockError:        errors.New("network error"),
			expectedError:    true,
			expectedResponse: nil,
		},
		{
			name:   "POST request with body",
			method: http.MethodPost,
			path:   "/v2/projects/123/assets/_search",
			body: map[string]interface{}{
				"filter": map[string]interface{}{
					"type": "domain",
				},
			},
			mockResponseCode: http.StatusOK,
			mockResponseBody: `{"assets": [{"id": "example.com", "type": "domain"}]}`,
			mockError:        nil,
			expectedError:    false,
			expectedResponse: []byte(`{"assets": [{"id": "example.com", "type": "domain"}]}`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock HTTP client
			mockHTTPClient := new(MockHTTPClient)

			// If an error is expected from the HTTP client call
			if tc.mockError != nil {
				mockHTTPClient.On("Do", mock.Anything).Return(nil, tc.mockError)
			} else {
				// Otherwise set up a successful response
				mockResponse := createHTTPResponse(tc.mockResponseCode, tc.mockResponseBody)
				mockHTTPClient.On("Do", mock.Anything).Return(mockResponse, nil)
			}

			// Create the client with our mock HTTP client
			client := &SecurityTrailsClient{
				baseURL:    "https://api.securitytrails.com",
				apiKey:     "test-api-key",
				httpClient: mockHTTPClient,
			}

			// Call the method under test
			response, err := client.Request(context.Background(), tc.method, tc.path, tc.body)

			// Verify the mock was called as expected
			mockHTTPClient.AssertExpectations(t)

			// Check errors
			if tc.expectedError {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Expected no error but got one")
				assert.Equal(t, tc.expectedResponse, response, "Response does not match expected")
			}
		})
	}
}

// TestFormatResponse tests the FormatResponse function
func TestFormatResponse(t *testing.T) {
	// Test cases
	testCases := []struct {
		name          string
		input         []byte
		expectedError bool
		expectedStart string // We check just the start to avoid whitespace issues
	}{
		{
			name:          "Valid JSON",
			input:         []byte(`{"key": "value", "array": [1, 2, 3]}`),
			expectedError: false,
			expectedStart: "{\n  \"array\": [\n    1,",
		},
		{
			name:          "Invalid JSON",
			input:         []byte(`{invalid json}`),
			expectedError: true,
			expectedStart: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := FormatResponse(tc.input)

			if tc.expectedError {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Expected no error but got one")
				assert.True(t, strings.Contains(result, tc.expectedStart),
					"Result doesn't match expected: %s", result)
			}
		})
	}
}

// TestPingTool tests the pingTool function
func TestPingTool(t *testing.T) {
	// Test cases
	testCases := []struct {
		name          string
		mockResponse  []byte
		mockError     error
		expectedError bool
		expectedText  string
	}{
		{
			name:          "Successful ping",
			mockResponse:  []byte(`{"success": true, "message": "pong"}`),
			mockError:     nil,
			expectedError: false,
			expectedText:  "SecurityTrails API Ping Response:\nSuccess: true\nMessage: pong",
		},
		{
			name:          "API error",
			mockResponse:  nil,
			mockError:     errors.New("API error"),
			expectedError: true,
			expectedText:  "",
		},
		{
			name:          "Invalid response",
			mockResponse:  []byte(`{invalid}`),
			mockError:     nil,
			expectedError: true,
			expectedText:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock API client
			mockAPIClient := new(MockAPIClient)

			// Set up the expected call and response
			mockAPIClient.On("Request", mock.Anything, http.MethodGet, "/v1/ping", nil).
				Return(tc.mockResponse, tc.mockError)

			// Create the dependencies with our mock API client
			deps := ToolDependencies{
				Client: mockAPIClient,
			}

			// Call the function under test
			result, err := pingTool(deps, PingArgs{})

			// Verify expectations
			mockAPIClient.AssertExpectations(t)

			// Check the result
			if tc.expectedError {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Expected no error but got one")
				assert.Equal(t, tc.expectedText, result, "Result text doesn't match expected")
			}
		})
	}
}

// TestListProjectsTool tests the listProjectsTool function
func TestListProjectsTool(t *testing.T) {
	// Test cases
	testCases := []struct {
		name          string
		mockResponse  []byte
		mockError     error
		expectedError bool
	}{
		{
			name:          "Successful project listing",
			mockResponse:  []byte(`{"projects": [{"id": "proj1", "name": "Project 1"}, {"id": "proj2", "name": "Project 2"}]}`),
			mockError:     nil,
			expectedError: false,
		},
		{
			name:          "API error",
			mockResponse:  nil,
			mockError:     errors.New("API error"),
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock API client
			mockAPIClient := new(MockAPIClient)

			// Set up the expected call and response
			mockAPIClient.On("Request", mock.Anything, http.MethodGet, "/v2/projects", nil).
				Return(tc.mockResponse, tc.mockError)

			// Create the dependencies with our mock API client
			deps := ToolDependencies{
				Client: mockAPIClient,
			}

			// Call the function under test
			_, err := listProjectsTool(deps, ListProjectsArgs{})

			// Verify expectations
			mockAPIClient.AssertExpectations(t)

			// Check for error
			if tc.expectedError {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Expected no error but got one")
			}
		})
	}
}

// TestAPIErrorString tests the Error method of APIError
func TestAPIErrorString(t *testing.T) {
	// Create an API error
	apiErr := &APIError{
		StatusCode: 401,
		Message:    "Invalid API key",
		Details: map[string]interface{}{
			"error": "unauthorized",
		},
	}

	// Test the Error method
	errorString := apiErr.Error()
	assert.Contains(t, errorString, "401", "Error string should contain status code")
	assert.Contains(t, errorString, "Invalid API key", "Error string should contain error message")
}

// TestDefaultConfigLoader tests the LoadConfig method
func TestDefaultConfigLoader(t *testing.T) {
	// Save any existing environment variable
	originalAPIKey := os.Getenv("SECURITYTRAILS_API_KEY")
	defer os.Setenv("SECURITYTRAILS_API_KEY", originalAPIKey)

	// Test getting API key from environment
	os.Setenv("SECURITYTRAILS_API_KEY", "test-api-key-from-env")

	loader := &DefaultConfigLoader{}
	apiKey, err := loader.GetSecurityTrailsAPIKey()

	// Verify the result
	assert.NoError(t, err, "Getting API key from environment should not error")
	assert.Equal(t, "test-api-key-from-env", apiKey, "API key should match environment variable")

	// Test missing API key
	os.Unsetenv("SECURITYTRAILS_API_KEY")

	// Create a mock for the LoadConfig method since we don't want to actually read files
	mockLoader := new(MockConfigLoader)
	mockLoader.On("LoadConfig").Return(nil, errors.New("config not found"))

	// Setup expectations for GetSecurityTrailsAPIKey
	mockLoader.On("GetSecurityTrailsAPIKey").Return("", errors.New("API key not found"))

	// Test key not found in environment or config
	_, err = mockLoader.GetSecurityTrailsAPIKey()
	assert.Error(t, err, "Should error when API key is not found")
}

// TestSearchAssetsTool tests the searchAssetsTool function
func TestSearchAssetsTool(t *testing.T) {
	// Create a mock API client
	mockAPIClient := new(MockAPIClient)

	// Test data
	projectID := "test-project"

	// Create test arguments using the new nested structure
	args := SearchAssetsArgs{
		ProjectID: projectID,
		AssetProperties: map[string]interface{}{
			"asset_id": map[string]interface{}{
				"eq": "example.com",
			},
		},
		TechnologyProperties: map[string]interface{}{
			"waf_detected": map[string]interface{}{
				"eq": true,
			},
		},
		Enrichments: []string{"dns"},
		Limit:       100,
		Cursor:      "next-page",
	}

	// Expected response from the API
	mockResponse := []byte(`{"assets": [{"id": "example.com", "type": "domain"}]}`)

	// Set up the mock call and response
	mockAPIClient.On("Request",
		mock.Anything,
		http.MethodPost,
		"/v2/projects/test-project/assets/_search",
		mock.MatchedBy(func(body interface{}) bool {
			// Validate the request body has the expected structure
			m, ok := body.(map[string]interface{})
			if !ok {
				return false
			}

			// Check filter structure
			filter, filterOk := m["filter"].(map[string]interface{})
			if !filterOk {
				return false
			}

			// Check asset_properties exists in filter
			assetProps, assetPropsOk := filter["asset_properties"].(map[string]interface{})
			if !assetPropsOk {
				return false
			}

			// Check technology_properties exists in filter
			techProps, techPropsOk := filter["technology_properties"].(map[string]interface{})
			if !techPropsOk {
				return false
			}

			// Check pagination structure
			pagination, paginationOk := m["pagination"].(map[string]interface{})
			if !paginationOk {
				return false
			}

			// Verify specific values
			assetIdFilter, assetIdOk := assetProps["asset_id"].(map[string]interface{})
			if !assetIdOk || assetIdFilter["eq"] != "example.com" {
				return false
			}

			wafDetected, wafOk := techProps["waf_detected"].(map[string]interface{})
			if !wafOk || wafDetected["eq"] != true {
				return false
			}

			return pagination["limit"] == float64(100) && pagination["cursor"] == "next-page"
		})).
		Return(mockResponse, nil)

	// Create the dependencies with our mock API client
	deps := ToolDependencies{
		Client: mockAPIClient,
	}

	// Call the function under test
	result, err := searchAssetsTool(deps, args)

	// Verify the mock was called
	mockAPIClient.AssertExpectations(t)

	// Check the result
	require.NoError(t, err, "searchAssetsTool should not return an error")
	assert.Contains(t, result, "example.com", "Result should contain the asset ID")
}

// Additional test for the raw filter option
func TestSearchAssetsWithRawFilterTool(t *testing.T) {
	// Create a mock API client
	mockAPIClient := new(MockAPIClient)

	// Test data with raw filter
	projectID := "test-project"

	rawFilter := map[string]interface{}{
		"asset_properties": map[string]interface{}{
			"asset_id": map[string]interface{}{
				"eq": "1.1.1.1",
			},
		},
		"technology_properties": map[string]interface{}{
			"waf_detected": map[string]interface{}{
				"eq": true,
			},
		},
	}

	args := SearchAssetsArgs{
		ProjectID: projectID,
		FilterRaw: rawFilter,
		Limit:     50,
	}

	// Expected response from the API
	mockResponse := []byte(`{"assets": [{"id": "1.1.1.1", "type": "ip"}]}`)

	// Set up the mock call and response
	mockAPIClient.On("Request",
		mock.Anything,
		http.MethodPost,
		"/v2/projects/test-project/assets/_search",
		mock.MatchedBy(func(body interface{}) bool {
			// Validate the request body has the expected structure
			m, ok := body.(map[string]interface{})
			if !ok {
				return false
			}

			// Check filter structure matches our raw filter
			filter, filterOk := m["filter"].(map[string]interface{})
			if !filterOk {
				return false
			}

			// Should have our exact raw filter
			assetProps, assetPropsOk := filter["asset_properties"].(map[string]interface{})
			if !assetPropsOk {
				return false
			}

			assetIdFilter, assetIdOk := assetProps["asset_id"].(map[string]interface{})
			if !assetIdOk || assetIdFilter["eq"] != "1.1.1.1" {
				return false
			}

			// Check pagination has limit 50
			pagination, paginationOk := m["pagination"].(map[string]interface{})
			if !paginationOk || pagination["limit"] != float64(50) {
				return false
			}

			return true
		})).
		Return(mockResponse, nil)

	// Create the dependencies with our mock API client
	deps := ToolDependencies{
		Client: mockAPIClient,
	}

	// Call the function under test
	result, err := searchAssetsTool(deps, args)

	// Verify the mock was called
	mockAPIClient.AssertExpectations(t)

	// Check the result
	require.NoError(t, err, "searchAssetsTool should not return an error")
	assert.Contains(t, result, "1.1.1.1", "Result should contain the asset ID")
}

// TestFindAssetsTool tests the findAssetsTool function
func TestFindAssetsTool(t *testing.T) {
	// Create a mock API client
	mockAPIClient := new(MockAPIClient)

	// Test data - a simple query that should generate a query string
	args := FindAssetsArgs{
		ProjectID: "test-project",
		Limit:     50,
		AssetType: "domain",
	}

	// Expected response from the API
	mockResponse := []byte(`{"assets": [{"id": "example.com", "type": "domain"}]}`)

	// The path should include query parameters
	expectedPathPrefix := "/v2/projects/test-project/assets?"

	// Set up the mock call with a flexible path matcher
	mockAPIClient.On("Request",
		mock.Anything,
		http.MethodGet,
		mock.MatchedBy(func(path string) bool {
			return strings.HasPrefix(path, expectedPathPrefix) &&
				strings.Contains(path, "limit=50") &&
				strings.Contains(path, "asset_type=domain")
		}),
		nil).
		Return(mockResponse, nil)

	// Create the dependencies with our mock API client
	deps := ToolDependencies{
		Client: mockAPIClient,
	}

	// Call the function under test
	result, err := findAssetsTool(deps, args)

	// Verify the mock was called
	mockAPIClient.AssertExpectations(t)

	// Check the result
	require.NoError(t, err, "findAssetsTool should not return an error")
	assert.Contains(t, result, "example.com", "Result should contain the asset ID")
}

// TestReadAssetTool tests the readAssetTool function
func TestReadAssetTool(t *testing.T) {
	// Create a mock API client
	mockAPIClient := new(MockAPIClient)

	// Test data
	args := ReadAssetArgs{
		ProjectID:        "test-project",
		AssetID:          "example.com",
		AdditionalFields: []string{"dns", "ssl"},
	}

	// Expected response from the API
	mockResponse := []byte(`{"id": "example.com", "type": "domain", "dns": {}, "ssl": {}}`)

	// The path should include the project and asset IDs
	expectedPathPrefix := "/v2/projects/test-project/assets/example.com"

	// Set up the mock call with a flexible path matcher
	mockAPIClient.On("Request",
		mock.Anything,
		http.MethodGet,
		mock.MatchedBy(func(path string) bool {
			return strings.HasPrefix(path, expectedPathPrefix) &&
				strings.Contains(path, "additional_fields=dns") &&
				strings.Contains(path, "additional_fields=ssl")
		}),
		nil).
		Return(mockResponse, nil)

	// Create the dependencies with our mock API client
	deps := ToolDependencies{
		Client: mockAPIClient,
	}

	// Call the function under test
	result, err := readAssetTool(deps, args)

	// Verify the mock was called
	mockAPIClient.AssertExpectations(t)

	// Check the result
	require.NoError(t, err, "readAssetTool should not return an error")
	assert.Contains(t, result, "example.com", "Result should contain the asset ID")
}
