package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// #233. The shape guards pin the *objects* inside a response — Forum, ForumTopic,
// ForumPost, ForumSearchResult — and nothing pinned the envelope around them.
//
// That is not a small gap. The envelope is what an integrator destructures first, and
// it carries the fields most likely to be quietly added or renamed: `total`, `page`,
// `per_page`, `can_moderate`, `can_create_topic`. A key added there, or one documented
// and never sent, was invisible to every existing test.
//
// Pinned by driving the real handler and comparing the top-level keys of the body
// against the inline response schema for that operation, in both directions. Handler
// level rather than builder level because an envelope only exists at the point the
// handler assembles it.
func TestForumResponseEnvelopesMatchTheSpec(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   string // as written in openapi.yaml
		method string
		status string
		call   func(*forumDeps) *httptest.ResponseRecorder
	}{{
		name: "list forums", path: "/api/v1/forums", method: "get", status: "200",
		call: func(d *forumDeps) *httptest.ResponseRecorder {
			req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums", nil), 7, memberPerms())
			w := httptest.NewRecorder()
			d.handler().HandleListForums(w, req)
			return w
		},
	}, {
		name: "get forum", path: "/api/v1/forums/{id}", method: "get", status: "200",
		call: func(d *forumDeps) *httptest.ResponseRecorder {
			req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/1", nil), 7, memberPerms())
			req = withURLParam(req, "id", "1")
			w := httptest.NewRecorder()
			d.handler().HandleGetForum(w, req)
			return w
		},
	}, {
		name: "list topics", path: "/api/v1/forums/{id}/topics", method: "get", status: "200",
		call: func(d *forumDeps) *httptest.ResponseRecorder {
			req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/1/topics", nil), 7, memberPerms())
			req = withURLParam(req, "id", "1")
			w := httptest.NewRecorder()
			d.handler().HandleListTopics(w, req)
			return w
		},
	}, {
		name: "get topic", path: "/api/v1/forums/topics/{id}", method: "get", status: "200",
		call: func(d *forumDeps) *httptest.ResponseRecorder {
			d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, Title: "A topic"}
			req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100", nil), 7, memberPerms())
			req = withURLParam(req, "id", "100")
			w := httptest.NewRecorder()
			d.handler().HandleGetTopic(w, req)
			return w
		},
	}, {
		name: "search", path: "/api/v1/forums/search", method: "get", status: "200",
		call: func(d *forumDeps) *httptest.ResponseRecorder {
			req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/search?q=hello", nil), 7, memberPerms())
			w := httptest.NewRecorder()
			d.handler().HandleSearchForum(w, req)
			return w
		},
	}, {
		name: "create topic", path: "/api/v1/forums/{id}/topics", method: "post", status: "201",
		call: func(d *forumDeps) *httptest.ResponseRecorder {
			req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/1/topics",
				strings.NewReader(`{"title":"A new topic","body":"hello"}`)), 7, memberPerms())
			req = withURLParam(req, "id", "1")
			w := httptest.NewRecorder()
			d.handler().HandleCreateTopic(w, req)
			return w
		},
	}, {
		name: "create post", path: "/api/v1/forums/topics/{id}/posts", method: "post", status: "201",
		call: func(d *forumDeps) *httptest.ResponseRecorder {
			d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, Title: "A topic"}
			req := withForumAuth(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/100/posts",
				strings.NewReader(`{"body":"a reply"}`)), 7, memberPerms())
			req = withURLParam(req, "id", "100")
			w := httptest.NewRecorder()
			d.handler().HandleCreatePost(w, req)
			return w
		},
	}, {
		name: "edit post", path: "/api/v1/forums/posts/{id}", method: "put", status: "200",
		call: func(d *forumDeps) *httptest.ResponseRecorder {
			d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
			d.posts.posts[500] = &model.ForumPost{ID: 500, TopicID: 100, UserID: 7, Body: "old"}
			req := withForumAuth(httptest.NewRequest(http.MethodPut, "/api/v1/forums/posts/500",
				strings.NewReader(`{"body":"new"}`)), 7, memberPerms())
			req = withURLParam(req, "id", "500")
			w := httptest.NewRecorder()
			d.handler().HandleEditPost(w, req)
			return w
		},
	}, {
		name: "list post edits", path: "/api/v1/forums/posts/{id}/edits", method: "get", status: "200",
		call: func(d *forumDeps) *httptest.ResponseRecorder {
			d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7}
			d.posts.posts[500] = &model.ForumPost{ID: 500, TopicID: 100, UserID: 7, Body: "b"}
			req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/posts/500/edits", nil), 1, staffPerms())
			req = withURLParam(req, "id", "500")
			w := httptest.NewRecorder()
			d.handler().HandleListPostEdits(w, req)
			return w
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			documented := envelopeProperties(t, tc.path, tc.method, tc.status)
			if len(documented) == 0 {
				t.Fatalf("%s %s [%s] documents no inline envelope, so this test would "+
					"assert nothing", strings.ToUpper(tc.method), tc.path, tc.status)
			}

			w := tc.call(newForumDeps())
			if w.Code >= 300 {
				t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
			}
			body := decodeBody(t, w)

			for key := range body {
				if !documented[key] {
					t.Errorf("the response envelope carries %q, which %s %s does not "+
						"document — an integrator destructuring this body has no way to "+
						"know the field exists", key, strings.ToUpper(tc.method), tc.path)
				}
			}
			for key := range documented {
				if _, ok := body[key]; !ok {
					t.Errorf("%s %s documents envelope key %q, which the response never "+
						"sends — a client coding against it gets undefined",
						strings.ToUpper(tc.method), tc.path, key)
				}
			}
		})
	}
}

// envelopeProperties reads the inline response-schema properties for one operation.
//
// Parsed separately from the components section: envelopes are declared inline at the
// path, which is why the components-only parser the shape guards use could not see
// them, and why they went unpinned.
func envelopeProperties(t *testing.T, path, method, status string) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile(fullSpecPath)
	if err != nil {
		t.Fatalf("reading %s: %v", fullSpecPath, err)
	}

	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Properties map[string]yaml.Node `yaml:"properties"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", fullSpecPath, err)
	}

	op, ok := doc.Paths[path][method]
	if !ok {
		t.Fatalf("%s documents no %s %s", fullSpecPath, strings.ToUpper(method), path)
	}
	resp, ok := op.Responses[status]
	if !ok {
		t.Fatalf("%s %s documents no %s response", strings.ToUpper(method), path, status)
	}

	out := map[string]bool{}
	for name := range resp.Content["application/json"].Schema.Properties {
		out[name] = true
	}
	return out
}

// Names the endpoints this file covers, so a forum operation added later is either
// covered or its absence is a deliberate, visible decision. Without it the table
// above is just a list somebody remembered to extend.
func TestEveryForumEnvelopeIsCovered(t *testing.T) {
	raw, err := os.ReadFile(fullSpecPath)
	if err != nil {
		t.Fatalf("reading %s: %v", fullSpecPath, err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Type       string               `yaml:"type"`
						Properties map[string]yaml.Node `yaml:"properties"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	// Covered by the table above.
	covered := map[string]bool{
		"get /api/v1/forums": true, "get /api/v1/forums/{id}": true,
		"get /api/v1/forums/{id}/topics": true, "post /api/v1/forums/{id}/topics": true,
		"get /api/v1/forums/topics/{id}": true, "get /api/v1/forums/search": true,
		"post /api/v1/forums/topics/{id}/posts": true, "put /api/v1/forums/posts/{id}": true,
		"get /api/v1/forums/posts/{id}/edits": true,
	}

	var uncovered []string
	for path, methods := range doc.Paths {
		if !strings.Contains(path, "/forums") {
			continue
		}
		for method, op := range methods {
			for code, resp := range op.Responses {
				if !strings.HasPrefix(code, "2") {
					continue
				}
				schema := resp.Content["application/json"].Schema
				// Only operations that actually return an envelope.
				if schema.Type != "object" || len(schema.Properties) == 0 {
					continue
				}
				key := method + " " + path
				if !covered[key] {
					uncovered = append(uncovered, key)
				}
			}
		}
	}

	sort.Strings(uncovered)
	if len(uncovered) > 0 {
		t.Errorf("these forum operations return a documented envelope that nothing "+
			"pins:\n  %s\nAdd them to TestForumResponseEnvelopesMatchTheSpec, or the "+
			"envelope drifts unwatched.", strings.Join(uncovered, "\n  "))
	}
}
