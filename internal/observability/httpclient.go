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

// WithHeaders returns a shallow copy of the client with a transport that injects
// the provided headers if they are not set on the request already.
func WithHeaders(base *http.Client, headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return base
	}
	if base == nil {
		base = &http.Client{}
	}
	rt := base.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	c := *base
	c.Transport = &staticHeaderTransport{base: rt, headers: headers}
	return &c
}

type staticHeaderTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *staticHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, v := range t.headers {
		if r.Header.Get(k) == "" {
			r.Header.Set(k, v)
		}
	}
	return t.base.RoundTrip(r)
}
