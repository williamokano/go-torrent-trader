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

// ErrRedirect reports a redirect this client refused to follow.
//
// Deliberately not a network error: the site answered, and answered with a
// redirect it should not have sent. Classifying it as unreachable would have a
// cron wrapper retry "the outage" forever against a site that is up and
// misconfigured.
var ErrRedirect = errors.New("refused to follow redirect")

// maxRedirects matches net/http's own default cap.
const maxRedirects = 10

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
	// Fragment is "" for a trailing bare "#", so the parsed value alone misses
	// "https://site/#" — which is accepted, drops the fragment client-side, and
	// sends every request to "/" instead. A real SPA answers that with its shell
	// and a 200, producing `invalid character '<'` rather than naming the bad URL.
	// That is exactly the failure the check above exists to prevent, so match on
	// the character.
	if parsed.Fragment != "" || strings.Contains(baseURL, "#") {
		return nil, fmt.Errorf("site URL %q must not contain a fragment", baseURL)
	}
	// A path is deliberately *not* rejected: a site served from a subdirectory is a
	// supported deployment (TestBaseURLWithAPathPrefixIsPreserved). That does mean
	// pasting an endpoint URL yields "/api/v1/auth/me/api/v1/auth/me", which reads
	// badly — but there is no way to tell that apart from a legitimate subpath, and
	// breaking real installs to improve one error message is the wrong trade.
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
	// After the options, so WithHTTPClient cannot leave a client that follows
	// redirects across origins — including the one tests inject.
	c.http.CheckRedirect = refuseCrossOriginRedirect
	return c, nil
}

// refuseCrossOriginRedirect stops a redirect that would move the request to a
// different scheme, host or port.
//
// net/http's default policy strips the Authorization header only when the target
// *hostname* differs, ignoring both scheme and port. Two consequences, both
// reachable without anything hostile:
//
//   - a redirect to another port on the same host forwards the bearer token, so
//     any co-tenant process listening there collects a live credential;
//   - an https->http redirect to the same host forwards it in cleartext, which is
//     what a reverse proxy missing X-Forwarded-Proto produces — a routine
//     misconfiguration that makes an app emit http:// redirects.
//
// A CLI talking to one site's API has no legitimate need to follow a redirect off
// that origin, so this refuses rather than stripping the header: a request that
// silently loses its credential comes back as a confusing 401, while this names
// the problem. It applies whether or not a token is set, which also keeps a
// 307/308 from replaying a login body to somewhere else.
func refuseCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("%w: stopped after %d redirects", ErrRedirect, maxRedirects)
	}
	origin := via[0].URL
	if !sameOrigin(origin, req.URL) {
		return fmt.Errorf("%w: %s redirected to %s, a different origin — a bearer token must not cross origins",
			ErrRedirect, originOf(origin), originOf(req.URL))
	}
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		portOf(a) == portOf(b)
}

// portOf resolves the effective port, so https://host and https://host:443 are
// one origin rather than two.
func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func originOf(u *url.URL) string {
	return u.Scheme + "://" + u.Host
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
