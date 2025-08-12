package observability

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// NewHTTPClient returns an http.Client instrumented with otelhttp transport.
func NewHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	rt := base.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	base.Transport = otelhttp.NewTransport(rt)
	return base
}
