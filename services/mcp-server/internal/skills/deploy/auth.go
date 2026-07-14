package deploy

import "net/http"

// bearerRoundTripper injects an Authorization: Bearer header on every outbound
// HTTP request. It wraps an underlying RoundTripper (typically http.DefaultTransport).
type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

// newBearerHTTPClient returns an *http.Client whose transport attaches the
// provided bearer token to all requests. Used by the pre-flight MCP SSE client.
func newBearerHTTPClient(token string) *http.Client {
	base := http.DefaultTransport
	if base == nil {
		base = &http.Transport{}
	}
	return &http.Client{
		Transport: &bearerRoundTripper{
			token: token,
			base:  base,
		},
	}
}

// RoundTrip implements http.RoundTripper.
func (rt *bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+rt.token)
	return rt.base.RoundTrip(cloned)
}
