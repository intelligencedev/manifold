package apidocs

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"manifold/internal/version"
)

// Options controls OpenAPI generation behavior.
type Options struct {
	// ServerURL is the API base URL injected into the spec's servers list.
	// Example: http://localhost:32180
	ServerURL string
	// AuthEnabled controls whether cookie auth security metadata is included.
	AuthEnabled bool
	// AuthCookieName sets the cookie name used by session auth.
	AuthCookieName string
}

type routeSpec struct {
	path       string
	operations []operationSpec
}

type operationSpec struct {
	method       string
	tag          string
	summary      string
	description  string
	requestBody  string
	successCode  int
	responseMode string
	queryParams  []paramSpec
	requiresAuth bool
}

type paramSpec struct {
	name        string
	schemaType  string
	description string
	required    bool
}

type opOption func(*operationSpec)

var (
	pathParamRe    = regexp.MustCompile(`\{([^{}]+)\}`)
	operationIDSan = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

// GenerateSpecJSON returns an OpenAPI 3.1 JSON document.
func GenerateSpecJSON(opts Options) ([]byte, error) {
	spec := buildSpec(opts)
	return json.MarshalIndent(spec, "", "  ")
}

func buildSpec(opts Options) map[string]any {
	serverURL := strings.TrimSpace(opts.ServerURL)
	if serverURL == "" {
		serverURL = "http://localhost:32180"
	}
	cookieName := strings.TrimSpace(opts.AuthCookieName)
	if cookieName == "" {
		cookieName = "sio_session"
	}

	paths := map[string]any{}
	tagSet := map[string]struct{}{}
	for _, route := range routeCatalog() {
		pathItem := map[string]any{}
		for _, op := range route.operations {
			tagSet[op.tag] = struct{}{}
			method := strings.ToLower(op.method)
			operation := map[string]any{
				"operationId": operationID(op.method, route.path),
				"summary":     op.summary,
				"tags":        []string{op.tag},
				"responses":   buildResponses(op),
			}
			if op.description != "" {
				operation["description"] = op.description
			}
			if params := buildParameters(route.path, op.queryParams); len(params) > 0 {
				operation["parameters"] = params
			}
			if rb := buildRequestBody(op.requestBody); rb != nil {
				operation["requestBody"] = rb
			}
			if opts.AuthEnabled && op.requiresAuth {
				operation["security"] = []any{map[string]any{"sessionCookie": []any{}}}
			}
			pathItem[method] = operation
		}
		paths[route.path] = pathItem
	}

	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Manifold API",
			"version":     version.Version,
			"description": "HTTP API for Manifold agentd, workflows, projects, MCP, and playground services.",
		},
		"servers": []map[string]any{
			{
				"url":         serverURL,
				"description": "Primary Manifold API server",
			},
		},
		"paths": paths,
		"tags":  buildTags(tagSet),
		"components": map[string]any{
			"schemas": map[string]any{
				"GenericObject": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
				"Error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	if opts.AuthEnabled {
		components := spec["components"].(map[string]any)
		components["securitySchemes"] = map[string]any{
			"sessionCookie": map[string]any{
				"type":        "apiKey",
				"in":          "cookie",
				"name":        cookieName,
				"description": "Session cookie used by agentd authentication.",
			},
		}
	}

	return spec
}

func buildTags(tagSet map[string]struct{}) []map[string]any {
	tagDescriptions := map[string]string{
		"System":      "Health checks and runtime status.",
		"Docs":        "OpenAPI and interactive API docs endpoints.",
		"Auth":        "Authentication, user identity, and RBAC management.",
		"Projects":    "Project and workspace file management.",
		"Chat":        "Agent run and chat session APIs.",
		"Specialists": "Specialist and orchestrator configuration APIs.",
		"Teams":       "Specialist team composition APIs.",
		"Metrics":     "Token, trace, and log metrics APIs.",
		"Media":       "Audio and image media endpoints.",
		"MCP":         "Model Context Protocol server management APIs.",
		"Flow":        "Flow v2 APIs.",
		"Durable":     "Postgres-backed durable task APIs.",
		"Debug":       "Memory and observability debugging endpoints.",
		"Playground":  "Prompt, dataset, and experiment playground APIs.",
	}

	order := []string{
		"System",
		"Docs",
		"Auth",
		"Projects",
		"Chat",
		"Specialists",
		"Teams",
		"Metrics",
		"Media",
		"MCP",
		"Flow",
		"Durable",
		"Debug",
		"Playground",
	}

	tags := make([]map[string]any, 0, len(tagSet))
	for _, name := range order {
		if _, ok := tagSet[name]; !ok {
			continue
		}
		tags = append(tags, map[string]any{
			"name":        name,
			"description": tagDescriptions[name],
		})
	}
	return tags
}

func buildParameters(path string, query []paramSpec) []map[string]any {
	params := make([]map[string]any, 0, len(query)+2)
	for _, m := range pathParamRe.FindAllStringSubmatch(path, -1) {
		name := m[1]
		params = append(params, map[string]any{
			"name":        name,
			"in":          "path",
			"required":    true,
			"description": pathParamDescription(name),
			"schema":      map[string]any{"type": "string"},
		})
	}
	for _, q := range query {
		params = append(params, map[string]any{
			"name":        q.name,
			"in":          "query",
			"required":    q.required,
			"description": q.description,
			"schema":      map[string]any{"type": q.schemaType},
		})
	}
	return params
}

func pathParamDescription(name string) string {
	descriptions := map[string]string{
		"project_id":   "Project identifier.",
		"session_id":   "Chat session identifier.",
		"message_id":   "Chat message identifier.",
		"name":         "Resource name.",
		"id":           "Resource identifier.",
		"intent":       "Workflow intent name.",
		"workflow_id":  "Flow workflow identifier.",
		"run_id":       "Run identifier.",
		"task_id":      "Durable task identifier.",
		"specialist":   "Specialist name.",
		"promptID":     "Prompt identifier.",
		"datasetID":    "Dataset identifier.",
		"experimentID": "Experiment identifier.",
		"filename":     "Relative media filename.",
	}
	if desc, ok := descriptions[name]; ok {
		return desc
	}
	return "Path parameter."
}

func buildRequestBody(kind string) map[string]any {
	switch kind {
	case "json":
		return map[string]any{
			"required": false,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/GenericObject"},
				},
			},
		}
	case "multipart":
		return map[string]any{
			"required": true,
			"content": map[string]any{
				"multipart/form-data": map[string]any{
					"schema": map[string]any{
						"type":                 "object",
						"additionalProperties": true,
					},
				},
			},
		}
	case "form":
		return map[string]any{
			"required": true,
			"content": map[string]any{
				"application/x-www-form-urlencoded": map[string]any{
					"schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"state": map[string]any{"type": "string"},
							"code":  map[string]any{"type": "string"},
							"user":  map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	default:
		return nil
	}
}

func buildResponses(op operationSpec) map[string]any {
	status := op.successCode
	if status == 0 {
		status = defaultSuccessCode(op.method)
	}
	statusKey := strconv.Itoa(status)
	responses := map[string]any{}

	switch op.responseMode {
	case "none":
		responses[statusKey] = map[string]any{"description": http.StatusText(status)}
	case "html":
		responses[statusKey] = map[string]any{
			"description": http.StatusText(status),
			"content": map[string]any{
				"text/html": map[string]any{
					"schema": map[string]any{"type": "string"},
				},
			},
		}
	case "binary":
		responses[statusKey] = map[string]any{
			"description": http.StatusText(status),
			"content": map[string]any{
				"application/octet-stream": map[string]any{
					"schema": map[string]any{"type": "string", "format": "binary"},
				},
			},
		}
	case "sse":
		responses[statusKey] = map[string]any{
			"description": "JSON response by default; SSE when Accept: text/event-stream.",
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/GenericObject"},
				},
				"text/event-stream": map[string]any{
					"schema": map[string]any{"type": "string"},
				},
			},
		}
	default:
		responses[statusKey] = map[string]any{
			"description": http.StatusText(status),
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/GenericObject"},
				},
			},
		}
	}

	// Common error responses.
	for _, code := range []int{400, 401, 403, 404, 500} {
		key := strconv.Itoa(code)
		if _, exists := responses[key]; exists {
			continue
		}
		responses[key] = map[string]any{
			"description": http.StatusText(code),
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{"$ref": "#/components/schemas/Error"},
				},
			},
		}
	}
	return responses
}

func operationID(method, path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return strings.ToLower(method) + "_root"
	}
	parts := strings.Split(trimmed, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ReplaceAll(part, "{", "")
		part = strings.ReplaceAll(part, "}", "")
		part = operationIDSan.ReplaceAllString(part, "_")
		part = strings.Trim(part, "_")
		if part == "" {
			continue
		}
		cleaned = append(cleaned, strings.ToLower(part))
	}
	if len(cleaned) == 0 {
		return strings.ToLower(method) + "_root"
	}
	return strings.ToLower(method) + "_" + strings.Join(cleaned, "__")
}

func defaultSuccessCode(method string) int {
	switch method {
	case http.MethodPost:
		return http.StatusCreated
	case http.MethodDelete:
		return http.StatusNoContent
	default:
		return http.StatusOK
	}
}

func jsonOp(method, tag, summary string, requiresAuth bool, opts ...opOption) operationSpec {
	op := operationSpec{
		method:       method,
		tag:          tag,
		summary:      summary,
		successCode:  defaultSuccessCode(method),
		responseMode: "json",
		requiresAuth: requiresAuth,
	}
	for _, apply := range opts {
		apply(&op)
	}
	return op
}

func withDescription(desc string) opOption {
	return func(op *operationSpec) { op.description = desc }
}

func withRequestBody(kind string) opOption {
	return func(op *operationSpec) { op.requestBody = kind }
}

func withSuccess(code int) opOption {
	return func(op *operationSpec) { op.successCode = code }
}

func withResponseMode(mode string) opOption {
	return func(op *operationSpec) { op.responseMode = mode }
}

func withQuery(params ...paramSpec) opOption {
	return func(op *operationSpec) { op.queryParams = append(op.queryParams, params...) }
}

func qp(name, schemaType, description string, required bool) paramSpec {
	return paramSpec{
		name:        name,
		schemaType:  schemaType,
		description: description,
		required:    required,
	}
}
