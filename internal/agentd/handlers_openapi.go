package agentd

import (
	"net/http"
	"strings"

	"manifold/internal/apidocs"
)

const openAPIDocsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Manifold API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; background: #f7f9fc; font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif; }
    .toolbar { display: flex; gap: 12px; align-items: center; padding: 12px 16px; background: #0f172a; color: #e2e8f0; }
    .toolbar a { color: #93c5fd; text-decoration: none; }
    .toolbar input { width: 420px; max-width: 52vw; padding: 8px; border: 1px solid #475569; border-radius: 6px; background: #0b1220; color: #e2e8f0; }
    .toolbar button { border: 0; background: #2563eb; color: #fff; border-radius: 6px; padding: 8px 12px; cursor: pointer; }
  </style>
</head>
<body>
  <div class="toolbar">
    <strong>Manifold API</strong>
    <span>Spec:</span>
    <input id="spec-url" type="text" value="/openapi.json" />
    <button id="load">Load</button>
    <span>Tip: add <code>?server=http://localhost:32180</code> to override target server.</span>
  </div>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    const params = new URLSearchParams(window.location.search);
    const serverOverride = params.get("server");
    const specInput = document.getElementById("spec-url");
    const defaultSpec = params.get("spec") || "/openapi.json";
    specInput.value = defaultSpec;

    async function render(specURL) {
      const res = await fetch(specURL, { credentials: "include" });
      if (!res.ok) throw new Error("Failed to load spec: " + res.status);
      const spec = await res.json();
      if (serverOverride) {
        spec.servers = [{ url: serverOverride, description: "Overridden via query parameter" }];
      }
      SwaggerUIBundle({
        spec,
        dom_id: "#swagger-ui",
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis],
        layout: "BaseLayout",
        tryItOutEnabled: true,
        persistAuthorization: true
      });
    }

    document.getElementById("load").addEventListener("click", () => {
      const url = specInput.value.trim();
      if (url) {
        render(url).catch((err) => alert(err.message));
      }
    });

    render(defaultSpec).catch((err) => {
      document.getElementById("swagger-ui").innerHTML =
        '<pre style="padding:16px;color:#b91c1c">Failed to load OpenAPI spec:\n' + err.message + '</pre>';
    });
  </script>
</body>
</html>
`

func (a *app) openapiSpecHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		doc, err := apidocs.GenerateSpecJSON(apidocs.Options{
			ServerURL:      requestBaseURL(r),
			AuthEnabled:    a.cfg.Auth.Enabled,
			AuthCookieName: a.cfg.Auth.CookieName,
		})
		if err != nil {
			http.Error(w, "failed to generate openapi", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(doc)
	}
}

func (a *app) openapiDocsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(openAPIDocsHTML))
	}
}

func requestBaseURL(r *http.Request) string {
	if r == nil {
		return "http://localhost:32180"
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xfProto := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); xfProto != "" {
		scheme = strings.ToLower(xfProto)
	}
	host := strings.TrimSpace(r.Host)
	if xfHost := firstForwardedValue(r.Header.Get("X-Forwarded-Host")); xfHost != "" {
		host = xfHost
	}
	if host == "" {
		host = "localhost:32180"
	}
	return scheme + "://" + host
}

func firstForwardedValue(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
