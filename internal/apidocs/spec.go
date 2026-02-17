package apidocs

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
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
		"WARPP":       "Workflow (WARPP) APIs.",
		"Flow":        "Flow v2 APIs.",
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
		"WARPP",
		"Flow",
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
	name := strings.Trim(path, "/")
	if name == "" {
		name = "root"
	}
	name = strings.ReplaceAll(name, "{", "")
	name = strings.ReplaceAll(name, "}", "")
	name = operationIDSan.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	return strings.ToLower(method) + "_" + strings.ToLower(name)
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

func routeCatalog() []routeSpec {
	routes := []routeSpec{
		{path: "/healthz", operations: []operationSpec{
			jsonOp(http.MethodGet, "System", "Health check", false, withDescription("Simple liveness probe.")),
		}},
		{path: "/readyz", operations: []operationSpec{
			jsonOp(http.MethodGet, "System", "Readiness check", false, withDescription("Simple readiness probe.")),
		}},
		{path: "/openapi.json", operations: []operationSpec{
			jsonOp(http.MethodGet, "Docs", "OpenAPI spec", false, withDescription("OpenAPI JSON document for this server."), withSuccess(http.StatusOK)),
		}},
		{path: "/api/openapi.json", operations: []operationSpec{
			jsonOp(http.MethodGet, "Docs", "OpenAPI spec (API alias)", false, withDescription("Alias of /openapi.json."), withSuccess(http.StatusOK)),
		}},
		{path: "/api-docs", operations: []operationSpec{
			jsonOp(http.MethodGet, "Docs", "Interactive API docs", false, withDescription("Swagger UI page."), withResponseMode("html"), withSuccess(http.StatusOK)),
		}},
		{path: "/api/docs", operations: []operationSpec{
			jsonOp(http.MethodGet, "Docs", "Interactive API docs (API alias)", false, withDescription("Alias of /api-docs."), withResponseMode("html"), withSuccess(http.StatusOK)),
		}},
		{path: "/auth/login", operations: []operationSpec{
			jsonOp(http.MethodGet, "Auth", "Start login flow", false, withDescription("Initiates OIDC/OAuth2 login when auth is configured."), withSuccess(http.StatusFound), withResponseMode("none")),
		}},
		{path: "/auth/callback", operations: []operationSpec{
			jsonOp(http.MethodGet, "Auth", "Auth callback", false, withDescription("OIDC/OAuth2 callback endpoint."), withSuccess(http.StatusFound), withResponseMode("none")),
		}},
		{path: "/auth/logout", operations: []operationSpec{
			jsonOp(http.MethodGet, "Auth", "Logout", false, withDescription("Ends local session and may redirect to upstream IdP logout."), withSuccess(http.StatusFound), withResponseMode("none")),
		}},
		{path: "/api/me", operations: []operationSpec{
			jsonOp(http.MethodGet, "Auth", "Current user profile", true),
		}},
		{path: "/api/users", operations: []operationSpec{
			jsonOp(http.MethodGet, "Auth", "List users", true),
			jsonOp(http.MethodPost, "Auth", "Create user", true, withRequestBody("json"), withSuccess(http.StatusOK)),
		}},
		{path: "/api/users/{id}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Auth", "Get user", true),
			jsonOp(http.MethodPut, "Auth", "Update user", true, withRequestBody("json")),
			jsonOp(http.MethodDelete, "Auth", "Delete user", true, withResponseMode("none")),
		}},
		{path: "/api/status", operations: []operationSpec{
			jsonOp(http.MethodGet, "System", "Specialist status", true),
		}},
		{path: "/api/runs", operations: []operationSpec{
			jsonOp(http.MethodGet, "Metrics", "List recent runs", true),
		}},
		{path: "/api/metrics/tokens", operations: []operationSpec{
			jsonOp(http.MethodGet, "Metrics", "Token usage metrics", true, withQuery(
				qp("window", "string", "Lookback duration (e.g. 1h, 24h, 7d).", false),
				qp("windowSeconds", "integer", "Lookback window in seconds.", false),
			)),
		}},
		{path: "/api/metrics/traces", operations: []operationSpec{
			jsonOp(http.MethodGet, "Metrics", "Trace metrics", true, withQuery(
				qp("window", "string", "Lookback duration.", false),
				qp("windowSeconds", "integer", "Lookback in seconds.", false),
				qp("limit", "integer", "Maximum number of traces.", false),
			)),
		}},
		{path: "/api/metrics/logs", operations: []operationSpec{
			jsonOp(http.MethodGet, "Metrics", "Log metrics", true, withQuery(
				qp("window", "string", "Lookback duration.", false),
				qp("windowSeconds", "integer", "Lookback in seconds.", false),
				qp("limit", "integer", "Maximum number of logs.", false),
			)),
		}},
		{path: "/api/config/agentd", operations: []operationSpec{
			jsonOp(http.MethodGet, "System", "Get runtime config", true),
			jsonOp(http.MethodPost, "System", "Update runtime config", true, withRequestBody("json"), withSuccess(http.StatusOK)),
			jsonOp(http.MethodPut, "System", "Replace runtime config", true, withRequestBody("json"), withSuccess(http.StatusOK)),
			jsonOp(http.MethodPatch, "System", "Patch runtime config", true, withRequestBody("json"), withSuccess(http.StatusOK)),
		}},
		{path: "/agent/run", operations: []operationSpec{
			jsonOp(http.MethodPost, "Chat", "Run orchestrator agent", true, withRequestBody("json"), withSuccess(http.StatusOK), withResponseMode("sse"), withQuery(
				qp("specialist", "string", "Force a specific specialist.", false),
				qp("team", "string", "Route the run through a team orchestrator.", false),
				qp("group", "string", "Legacy alias of team.", false),
				qp("warpp", "boolean", "Route request through WARPP workflow execution.", false),
			)),
		}},
		{path: "/agent/vision", operations: []operationSpec{
			jsonOp(http.MethodPost, "Media", "Run vision prompt with uploaded images", true, withRequestBody("multipart"), withSuccess(http.StatusOK), withResponseMode("sse")),
		}},
		{path: "/api/prompt", operations: []operationSpec{
			jsonOp(http.MethodPost, "Chat", "Run prompt endpoint", true, withRequestBody("json"), withSuccess(http.StatusOK), withResponseMode("sse")),
		}},
		{path: "/audio/{filename}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Media", "Fetch generated audio file", false, withResponseMode("binary"), withSuccess(http.StatusOK)),
		}},
		{path: "/stt", operations: []operationSpec{
			jsonOp(http.MethodPost, "Media", "Speech-to-text transcription", true, withRequestBody("multipart"), withSuccess(http.StatusOK)),
		}},
		{path: "/api/me/preferences", operations: []operationSpec{
			jsonOp(http.MethodGet, "Projects", "Get user preferences", true),
			jsonOp(http.MethodPut, "Projects", "Update user preferences", true, withRequestBody("json"), withSuccess(http.StatusOK)),
		}},
		{path: "/api/me/preferences/project", operations: []operationSpec{
			jsonOp(http.MethodPost, "Projects", "Set active project", true, withRequestBody("json"), withSuccess(http.StatusOK)),
		}},
		{path: "/api/projects", operations: []operationSpec{
			jsonOp(http.MethodGet, "Projects", "List projects", true),
			jsonOp(http.MethodPost, "Projects", "Create project", true, withRequestBody("json"), withSuccess(http.StatusCreated)),
		}},
		{path: "/api/projects/{project_id}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Projects", "Get project root listing", true),
			jsonOp(http.MethodDelete, "Projects", "Delete project", true, withResponseMode("none"), withSuccess(http.StatusNoContent)),
		}},
		{path: "/api/projects/{project_id}/archive", operations: []operationSpec{
			jsonOp(http.MethodGet, "Projects", "Download project archive (.tar.gz)", true, withResponseMode("binary")),
		}},
		{path: "/api/projects/{project_id}/tree", operations: []operationSpec{
			jsonOp(http.MethodGet, "Projects", "List project tree entries", true, withQuery(
				qp("path", "string", "Directory path to list (default root).", false),
			)),
		}},
		{path: "/api/projects/{project_id}/files", operations: []operationSpec{
			jsonOp(http.MethodGet, "Projects", "Read file", true, withResponseMode("binary"), withQuery(
				qp("path", "string", "File path within the project.", true),
			)),
			jsonOp(http.MethodPost, "Projects", "Upload/create file", true, withRequestBody("multipart"), withSuccess(http.StatusCreated), withQuery(
				qp("path", "string", "Target directory path.", false),
				qp("name", "string", "File name.", false),
			)),
			jsonOp(http.MethodDelete, "Projects", "Delete file", true, withResponseMode("none"), withSuccess(http.StatusNoContent), withQuery(
				qp("path", "string", "File path to remove.", true),
			)),
		}},
		{path: "/api/projects/{project_id}/dirs", operations: []operationSpec{
			jsonOp(http.MethodPost, "Projects", "Create directory", true, withSuccess(http.StatusCreated), withResponseMode("none"), withQuery(
				qp("path", "string", "Directory path to create.", true),
			)),
		}},
		{path: "/api/projects/{project_id}/move", operations: []operationSpec{
			jsonOp(http.MethodPost, "Projects", "Move/rename path", true, withRequestBody("json"), withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/chat/sessions", operations: []operationSpec{
			jsonOp(http.MethodGet, "Chat", "List chat sessions", true),
			jsonOp(http.MethodPost, "Chat", "Create chat session", true, withRequestBody("json"), withSuccess(http.StatusCreated)),
		}},
		{path: "/api/chat/sessions/{session_id}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Chat", "Get chat session", true),
			jsonOp(http.MethodPatch, "Chat", "Rename chat session", true, withRequestBody("json"), withSuccess(http.StatusOK)),
			jsonOp(http.MethodDelete, "Chat", "Delete chat session", true, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/chat/sessions/{session_id}/messages", operations: []operationSpec{
			jsonOp(http.MethodGet, "Chat", "List chat messages", true, withQuery(
				qp("limit", "integer", "Optional message limit.", false),
			)),
			jsonOp(http.MethodDelete, "Chat", "Delete messages after marker", true, withResponseMode("none"), withSuccess(http.StatusNoContent), withQuery(
				qp("after", "string", "Delete messages after this message ID.", true),
				qp("inclusive", "boolean", "Include the marker message in delete.", false),
			)),
		}},
		{path: "/api/chat/sessions/{session_id}/messages/{message_id}", operations: []operationSpec{
			jsonOp(http.MethodDelete, "Chat", "Delete one chat message", true, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/chat/sessions/{session_id}/title", operations: []operationSpec{
			jsonOp(http.MethodPost, "Chat", "Generate/apply session title", true, withRequestBody("json"), withSuccess(http.StatusOK)),
		}},
		{path: "/api/specialists/defaults", operations: []operationSpec{
			jsonOp(http.MethodGet, "Specialists", "Get provider defaults", true),
		}},
		{path: "/api/specialists", operations: []operationSpec{
			jsonOp(http.MethodGet, "Specialists", "List specialists", true),
			jsonOp(http.MethodPost, "Specialists", "Create specialist", true, withRequestBody("json"), withSuccess(http.StatusCreated)),
		}},
		{path: "/api/specialists/{name}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Specialists", "Get specialist", true),
			jsonOp(http.MethodPut, "Specialists", "Update specialist", true, withRequestBody("json"), withSuccess(http.StatusOK)),
			jsonOp(http.MethodDelete, "Specialists", "Delete specialist", true, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/teams", operations: []operationSpec{
			jsonOp(http.MethodGet, "Teams", "List teams", true),
			jsonOp(http.MethodPost, "Teams", "Create team", true, withRequestBody("json"), withSuccess(http.StatusCreated)),
		}},
		{path: "/api/teams/{name}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Teams", "Get team", true),
			jsonOp(http.MethodPut, "Teams", "Update team", true, withRequestBody("json"), withSuccess(http.StatusOK)),
			jsonOp(http.MethodDelete, "Teams", "Delete team", true, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/teams/{name}/members/{specialist}", operations: []operationSpec{
			jsonOp(http.MethodPut, "Teams", "Add specialist to team", true, withSuccess(http.StatusNoContent), withResponseMode("none")),
			jsonOp(http.MethodDelete, "Teams", "Remove specialist from team", true, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/warpp/tools", operations: []operationSpec{
			jsonOp(http.MethodGet, "WARPP", "List tool schemas", true),
		}},
		{path: "/api/warpp/workflows", operations: []operationSpec{
			jsonOp(http.MethodGet, "WARPP", "List WARPP workflows", true),
		}},
		{path: "/api/warpp/workflows/{intent}", operations: []operationSpec{
			jsonOp(http.MethodGet, "WARPP", "Get WARPP workflow", true),
			jsonOp(http.MethodPut, "WARPP", "Create/update WARPP workflow", true, withRequestBody("json"), withSuccess(http.StatusOK)),
			jsonOp(http.MethodDelete, "WARPP", "Delete WARPP workflow", true, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/warpp/run", operations: []operationSpec{
			jsonOp(http.MethodPost, "WARPP", "Execute WARPP workflow", true, withRequestBody("json"), withSuccess(http.StatusOK)),
		}},
		{path: "/api/flows/v2/tools", operations: []operationSpec{
			jsonOp(http.MethodGet, "Flow", "List tool schemas for Flow v2", true),
		}},
		{path: "/api/flows/v2/workflows", operations: []operationSpec{
			jsonOp(http.MethodGet, "Flow", "List Flow v2 workflows", true),
		}},
		{path: "/api/flows/v2/workflows/{workflow_id}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Flow", "Get Flow v2 workflow", true),
			jsonOp(http.MethodPut, "Flow", "Create/update Flow v2 workflow", true, withRequestBody("json"), withSuccess(http.StatusOK)),
			jsonOp(http.MethodDelete, "Flow", "Delete Flow v2 workflow", true, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/flows/v2/validate", operations: []operationSpec{
			jsonOp(http.MethodPost, "Flow", "Validate Flow v2 workflow", true, withRequestBody("json"), withSuccess(http.StatusOK)),
		}},
		{path: "/api/flows/v2/run", operations: []operationSpec{
			jsonOp(http.MethodPost, "Flow", "Start Flow v2 run", true, withRequestBody("json"), withSuccess(http.StatusAccepted)),
		}},
		{path: "/api/flows/v2/runs/{run_id}/events", operations: []operationSpec{
			jsonOp(http.MethodGet, "Flow", "Get or stream Flow v2 run events", true, withSuccess(http.StatusOK), withResponseMode("sse")),
		}},
		{path: "/api/mcp/servers", operations: []operationSpec{
			jsonOp(http.MethodGet, "MCP", "List MCP servers", true),
			jsonOp(http.MethodPost, "MCP", "Create MCP server", true, withRequestBody("json"), withSuccess(http.StatusCreated)),
		}},
		{path: "/api/mcp/servers/{name}", operations: []operationSpec{
			jsonOp(http.MethodPut, "MCP", "Update MCP server", true, withRequestBody("json"), withSuccess(http.StatusOK)),
			jsonOp(http.MethodDelete, "MCP", "Delete MCP server", true, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/mcp/oauth/start", operations: []operationSpec{
			jsonOp(http.MethodPost, "MCP", "Start MCP OAuth flow", true, withRequestBody("json"), withSuccess(http.StatusOK)),
		}},
		{path: "/api/mcp/oauth/callback", operations: []operationSpec{
			jsonOp(http.MethodGet, "MCP", "MCP OAuth callback", false, withResponseMode("html"), withSuccess(http.StatusOK)),
		}},
		{path: "/debug/memory", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "Memory debug root", true),
		}},
		{path: "/debug/memory/sessions", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "List debug memory sessions", true),
		}},
		{path: "/debug/memory/sessions/{session_id}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "Get debug memory session detail", true),
		}},
		{path: "/debug/memory/entries", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "List debug memory entries", true, withQuery(
				qp("session_id", "string", "Session ID to inspect.", true),
				qp("limit", "integer", "Optional entry limit.", false),
			)),
		}},
		{path: "/debug/memory/plan", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "Get derived memory plan", true, withQuery(
				qp("session_id", "string", "Session ID to inspect.", true),
			)),
		}},
		{path: "/debug/memory/evolving", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "Get evolving memory debug info", true),
		}},
		{path: "/api/debug/memory", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "Memory debug root (API alias)", true),
		}},
		{path: "/api/debug/memory/sessions", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "List debug memory sessions (API alias)", true),
		}},
		{path: "/api/debug/memory/sessions/{session_id}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "Get debug memory session detail (API alias)", true),
		}},
		{path: "/api/debug/memory/entries", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "List debug memory entries (API alias)", true, withQuery(
				qp("session_id", "string", "Session ID to inspect.", true),
				qp("limit", "integer", "Optional entry limit.", false),
			)),
		}},
		{path: "/api/debug/memory/plan", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "Get derived memory plan (API alias)", true, withQuery(
				qp("session_id", "string", "Session ID to inspect.", true),
			)),
		}},
		{path: "/api/debug/memory/evolving", operations: []operationSpec{
			jsonOp(http.MethodGet, "Debug", "Get evolving memory debug info (API alias)", true),
		}},
		{path: "/api/v1/playground/prompts", operations: []operationSpec{
			jsonOp(http.MethodGet, "Playground", "List prompts", false, withQuery(
				qp("q", "string", "Prompt search query.", false),
				qp("tag", "string", "Filter by tag.", false),
				qp("page", "integer", "Page number.", false),
				qp("per_page", "integer", "Page size.", false),
			)),
			jsonOp(http.MethodPost, "Playground", "Create prompt", false, withRequestBody("json"), withSuccess(http.StatusCreated)),
		}},
		{path: "/api/v1/playground/prompts/{promptID}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Playground", "Get prompt", false),
			jsonOp(http.MethodDelete, "Playground", "Delete prompt", false, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/v1/playground/prompts/{promptID}/versions", operations: []operationSpec{
			jsonOp(http.MethodGet, "Playground", "List prompt versions", false),
			jsonOp(http.MethodPost, "Playground", "Create prompt version", false, withRequestBody("json"), withSuccess(http.StatusCreated)),
		}},
		{path: "/api/v1/playground/datasets", operations: []operationSpec{
			jsonOp(http.MethodGet, "Playground", "List datasets", false),
			jsonOp(http.MethodPost, "Playground", "Create dataset", false, withRequestBody("json"), withSuccess(http.StatusCreated)),
		}},
		{path: "/api/v1/playground/datasets/{datasetID}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Playground", "Get dataset", false),
			jsonOp(http.MethodPut, "Playground", "Update dataset", false, withRequestBody("json"), withSuccess(http.StatusOK)),
			jsonOp(http.MethodDelete, "Playground", "Delete dataset", false, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/v1/playground/experiments", operations: []operationSpec{
			jsonOp(http.MethodGet, "Playground", "List experiments", false),
			jsonOp(http.MethodPost, "Playground", "Create experiment", false, withRequestBody("json"), withSuccess(http.StatusCreated)),
		}},
		{path: "/api/v1/playground/experiments/{experimentID}", operations: []operationSpec{
			jsonOp(http.MethodGet, "Playground", "Get experiment", false),
			jsonOp(http.MethodDelete, "Playground", "Delete experiment", false, withSuccess(http.StatusNoContent), withResponseMode("none")),
		}},
		{path: "/api/v1/playground/experiments/{experimentID}/runs", operations: []operationSpec{
			jsonOp(http.MethodGet, "Playground", "List experiment runs", false),
			jsonOp(http.MethodPost, "Playground", "Start experiment run", false, withSuccess(http.StatusAccepted)),
		}},
		{path: "/api/v1/playground/runs/{runID}/results", operations: []operationSpec{
			jsonOp(http.MethodGet, "Playground", "List run results", false),
		}},
	}

	// Keep deterministic ordering if this list is ever appended conditionally.
	sort.SliceStable(routes, func(i, j int) bool {
		return routes[i].path < routes[j].path
	})
	return routes
}
