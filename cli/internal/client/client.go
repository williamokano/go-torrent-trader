// Package client is a hand-written REST client for the site API.
//
// It is deliberately not generated from backend/api/openapi.yaml. Generation is
// the better answer once the spec is complete, but 152 of 189 routes are
// currently undocumented (#155), so generating today would produce a client with
// holes in it and no signal about where they are. The types here are the CLI's
// own — the shared-nothing rule in docs/ARCHITECTURE.md forbids importing
// backend/internal, and the migration tool sets the same precedent.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNoCredentials reports that a command needing authentication was invoked
// without a token.
var ErrNoCredentials = errors.New("no credentials configured")

// DefaultTimeout bounds a single API call.
const DefaultTimeout = 30 * time.Second

// maxErrorBodyBytes bounds how much of an unparseable error body is quoted back.
// A misconfigured URL that lands on a proxy returns an HTML page, and echoing all
// of it into a terminal buries the actual problem.
const maxErrorBodyBytes = 512

// Client talks to one site.
type Client struct {
	baseURL   string
	token     string
	userAgent string
	http      *http.Client
}

// Option customises a Client.
type Option func(*Client)

// WithHTTPClient replaces the underlying HTTP client. Tests use this to point at
// an httptest server with no timeout surprises.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithUserAgent sets the User-Agent header, which is how an operator reading
// access logs tells CLI traffic from browser traffic.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// New builds a client for anonymous requests.
func New(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing site URL %q: %w", baseURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("site URL %q must be http or https", baseURL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("site URL %q has no host", baseURL)
	}
	// A base URL is a site root, not a request. Anything after the path is a
	// copy-paste artefact, and concatenating a path onto it silently sends every
	// request somewhere else: "https://site/?utm=x" + "/api/v1/auth/me" asks for
	// "/?utm=x/api/v1/auth/me", which a real site answers with its SPA shell and
	// a 200. Rejecting it names the problem instead of yielding a decode error.
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return nil, fmt.Errorf("site URL %q must not contain a query string", baseURL)
	}
	if parsed.Fragment != "" {
		return nil, fmt.Errorf("site URL %q must not contain a fragment", baseURL)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("site URL %q must not contain credentials: pass --token or set TT_TOKEN", baseURL)
	}

	c := &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		userAgent: "tt",
		http:      &http.Client{Timeout: DefaultTimeout},
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// NewAuthenticated builds a client that carries a bearer token, refusing to
// build one at all when the token is empty.
//
// The refusal is the point. Sending the request anonymously would let the server
// answer with public data for some endpoints, so a missing credential would look
// like success with less data rather than a configuration error — lessons.md
// records that an absent dependency must never mean "allow".
func NewAuthenticated(baseURL, token string, opts ...Option) (*Client, error) {
	if token == "" {
		return nil, ErrNoCredentials
	}
	c, err := New(baseURL, opts...)
	if err != nil {
		return nil, err
	}
	c.token = token
	return c, nil
}

// APIError is a non-2xx response from the site.
type APIError struct {
	StatusCode int
	// Code is the machine-readable error code from the API envelope, empty when
	// the response was not the expected shape.
	Code string
	// Message is the human-readable message, or a truncated raw body when the
	// response could not be parsed.
	Message string
}

func (e *APIError) Error() string {
	switch {
	case e.Code != "" && e.Message != "":
		return fmt.Sprintf("%s (HTTP %d %s)", e.Message, e.StatusCode, e.Code)
	case e.Message != "":
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.StatusCode)
	default:
		return fmt.Sprintf("HTTP %d %s", e.StatusCode, http.StatusText(e.StatusCode))
	}
}

// errorEnvelope matches the backend's error shape: {"error":{"code","message"}}.
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Do performs a request against a path such as "/api/v1/auth/me".
//
// A non-nil body is JSON-encoded. A non-nil out is JSON-decoded from the
// response. Non-2xx responses become *APIError.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newAPIError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding %s %s response: %w", method, path, err)
	}
	return nil
}

// Get is Do with method GET and no request body.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.Do(ctx, http.MethodGet, path, nil, out)
}

func newAPIError(resp *http.Response) error {
	apiErr := &APIError{StatusCode: resp.StatusCode}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes+1))
	if err != nil {
		return apiErr
	}

	var env errorEnvelope
	if json.Unmarshal(raw, &env) == nil && env.Error.Message != "" {
		apiErr.Code = env.Error.Code
		apiErr.Message = env.Error.Message
		return apiErr
	}

	// Not the API envelope — a proxy, a wrong URL, or an unexpected shape. Quote
	// a bounded prefix so the user can see what actually answered.
	if trimmed := strings.TrimSpace(string(raw)); trimmed != "" {
		if len(trimmed) > maxErrorBodyBytes {
			trimmed = trimmed[:maxErrorBodyBytes] + "..."
		}
		apiErr.Message = trimmed
	}
	return apiErr
}
