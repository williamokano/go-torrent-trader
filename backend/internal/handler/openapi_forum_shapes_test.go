package handler

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// The forum responses are `map[string]interface{}` literals, so nothing but this
// test connects them to the schemas that describe them.
//
// That gap is the whole difficulty #155 records: response bodies "must be written by
// hand against the handler code. They cannot be derived." Written by hand, they are
// correct exactly once — on the day they are written. The next person to add a field
// to postResponse has no reason to look at openapi.yaml, and the spec quietly starts
// lying about the endpoint most integrators will hit first.
//
// So the builders are pinned to their schemas by key set, in both directions: a field
// added to the response without a schema entry fails, and a schema property with no
// field behind it fails too. It is not a full type check — see the limits recorded at
// the bottom of this file — but it catches the drift that actually happens, and it
// makes documenting the next domain cheaper rather than more expensive.
//
// One rule this file learned the hard way: never assert a schema against a
// hand-typed list of field names. The first version of the ForumPostEdit test did
// exactly that, and it passed while the schema was missing `reconstruction_failed`
// — a field the frontend was already rendering. A literal written from memory
// agrees with a schema written from the same memory, in the same sitting, and
// proves nothing. Derive the expectation from the code or don't assert it.

// schemaProperties reads the declared property names of one component schema.
//
// The spec is parsed once per test binary rather than once per call; there are
// several callers and the document is a few thousand lines.
func schemaProperties(t *testing.T, schema string) []string {
	t.Helper()

	doc := parsedFullSpec(t)
	s, ok := doc.Components.Schemas[schema]
	if !ok {
		t.Fatalf("%s documents no schema named %q", fullSpecPath, schema)
	}
	if len(s.Properties) == 0 {
		t.Fatalf("schema %q declares no properties", schema)
	}

	names := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type specSchemaDoc struct {
	Components struct {
		Schemas map[string]struct {
			Properties map[string]yaml.Node `yaml:"properties"`
		} `yaml:"schemas"`
	} `yaml:"components"`
}

var (
	cachedSpec    *specSchemaDoc
	cachedSpecErr error
)

func parsedFullSpec(t *testing.T) *specSchemaDoc {
	t.Helper()

	if cachedSpec == nil && cachedSpecErr == nil {
		raw, err := os.ReadFile(fullSpecPath)
		if err != nil {
			cachedSpecErr = err
		} else {
			var doc specSchemaDoc
			if err := yaml.Unmarshal(raw, &doc); err != nil {
				cachedSpecErr = err
			} else {
				cachedSpec = &doc
			}
		}
	}
	if cachedSpecErr != nil {
		t.Fatalf("reading %s: %v", fullSpecPath, cachedSpecErr)
	}
	return cachedSpec
}

// keySetOpt relaxes assertSameKeys for a documented property the shape under test
// legitimately omits.
type keySetOpt func(map[string]bool)

// omits declares that this shape does not carry the named properties, and that
// their absence is the documented behaviour rather than drift.
//
// Declarative on purpose. The earlier version of the deleted-post test wrote the
// missing key back into the map before asserting — which worked, but the next
// staff-only field would have been met with a second fabrication line, and each
// one silently weakens the member-side assertion it is bypassing.
func omits(keys ...string) keySetOpt {
	return func(allowed map[string]bool) {
		for _, k := range keys {
			allowed[k] = true
		}
	}
}

// assertSameKeys compares a response body's keys with a schema's properties.
func assertSameKeys(t *testing.T, schema string, body map[string]interface{}, opts ...keySetOpt) {
	t.Helper()

	mayOmit := map[string]bool{}
	for _, opt := range opts {
		opt(mayOmit)
	}

	documented := schemaProperties(t, schema)
	inSchema := make(map[string]bool, len(documented))
	for _, name := range documented {
		inSchema[name] = true
	}

	present := make(map[string]bool, len(body))
	for key := range body {
		present[key] = true
		if !inSchema[key] {
			t.Errorf("the response emits %q but schema %s does not document it — "+
				"the spec is now wrong about this endpoint", key, schema)
		}
	}

	for _, name := range documented {
		if present[name] || mayOmit[name] {
			continue
		}
		t.Errorf("schema %s documents %q, but the response never emits it — "+
			"an integrator would code against a field that is never sent", schema, name)
	}

	// A tolerance that stops being needed is itself drift: it means the shape now
	// emits the key and the exemption is hiding whatever happens next.
	for key := range mayOmit {
		if present[key] {
			t.Errorf("%q is declared omitted from this shape but the response emits it — "+
				"drop the omits() for it", key)
		}
	}
}

// wireKeys returns the JSON object keys a struct marshals to.
//
// Derived by reflection rather than listed, which is the point: a new serialised
// field on the model fails this test until it is documented. `json:"-"` is skipped
// because it never reaches the wire; `omitempty` is *not* skipped, because the key
// still appears whenever the value is non-zero and the spec has to describe it.
// A field with no tag at all is reported under its Go name, since that is what
// encoding/json would emit.
func wireKeys(t *testing.T, v interface{}) []string {
	t.Helper()

	rt := reflect.TypeOf(v)
	for rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		t.Fatalf("wireKeys wants a struct, got %s", rt.Kind())
	}

	var keys []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.PkgPath != "" {
			continue // unexported, never marshalled
		}
		if f.Anonymous {
			t.Fatalf("%s embeds %s; wireKeys does not flatten embedded structs",
				rt.Name(), f.Type)
		}
		tag, ok := f.Tag.Lookup("json")
		switch {
		case !ok:
			keys = append(keys, f.Name)
		case tag == "-":
			continue
		default:
			name := strings.Split(tag, ",")[0]
			if name == "" {
				name = f.Name
			}
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	return keys
}

// TestForumResponseMatchesSchema pins the forum body. Every optional field is set,
// because forumResponse only includes them when non-nil and a fixture that left them
// out would exercise the smaller of the two shapes.
func TestForumResponseMatchesSchema(t *testing.T) {
	at := time.Now()
	username := "member"
	topicID := int64(7)
	title := "A topic"

	forum := &model.Forum{
		ID: 1, CategoryID: 2, Name: "General", Description: "Chat",
		SortOrder: 1, TopicCount: 3, PostCount: 9,
		MinGroupLevel: 10, MinPostLevel: 20, CreatedAt: at,
		LastPostAt: &at, LastPostUsername: &username,
		LastPostTopicID: &topicID, LastPostTopicTitle: &title,
	}

	assertSameKeys(t, "Forum", forumResponse(forum))
}

func TestForumTopicResponseMatchesSchema(t *testing.T) {
	at := time.Now()
	username := "member"

	topic := &model.ForumTopic{
		ID: 1, ForumID: 2, ForumName: "General", UserID: 3, Username: "author",
		Title: "A topic", Pinned: true, Locked: true, PostCount: 4, ViewCount: 5,
		CreatedAt: at, UpdatedAt: at,
		LastPostAt: &at, LastPostUsername: &username,
	}

	assertSameKeys(t, "ForumTopic", topicResponse(topic))
}

// The post body has two shapes: what staff see on a deleted post, and what everyone
// else sees. Both are checked, because `deleted_by` is present in only one of them
// and a schema that documented just the smaller shape would omit it.
func TestForumPostResponseMatchesSchema(t *testing.T) {
	at := time.Now()
	replyTo := int64(9)
	avatar := "a.png"
	// User ids, not usernames — the response passes the model's *int64 straight
	// through. Writing this fixture is what caught the schema claiming otherwise.
	editedBy := int64(4)
	deletedBy := int64(5)

	post := &model.ForumPost{
		ID: 1, TopicID: 2, UserID: 3, Username: "author",
		Avatar:    &avatar,
		GroupName: "User", Body: "hello", MentionedUsernames: []string{"someone"},
		CreatedAt: at, UserCreatedAt: at, UserPostCount: 12, IsFirstPost: true,
		ReplyToPostID: &replyTo,
		EditedAt:      &at, EditedBy: &editedBy,
		DeletedAt: &at, DeletedBy: &deletedBy,
	}

	t.Run("as staff see a deleted post", func(t *testing.T) {
		assertSameKeys(t, "ForumPost", postResponse(post, true))
	})

	// A member sees every key except deleted_by, with body and mentions blanked
	// rather than removed.
	t.Run("as a member sees a deleted post", func(t *testing.T) {
		body := postResponse(post, false)

		// Stated separately from the omits() below even though that would also
		// catch it. This one is the privacy rule — a member is told their post was
		// removed, not which staff member removed it — and it should fail with a
		// message about disclosure rather than about schema bookkeeping.
		if _, present := body["deleted_by"]; present {
			t.Error("deleted_by reached a non-staff viewer; it identifies the moderator")
		}

		if body["body"] != "" {
			t.Errorf("body = %q, want it blanked for a non-staff viewer", body["body"])
		}
		// Asserted through the marshaller rather than against nil. The map holds a
		// nil []string inside an interface, which is famously not == nil in Go; what
		// matters to a client is that it arrives as JSON null, and that is the only
		// way to check it without repeating the bug.
		mentions, err := json.Marshal(body["mentioned_usernames"])
		if err != nil {
			t.Fatalf("marshaling mentioned_usernames: %v", err)
		}
		if string(mentions) != "null" {
			t.Errorf("mentioned_usernames marshals to %s, want null for a non-staff viewer",
				mentions)
		}

		assertSameKeys(t, "ForumPost", body, omits("deleted_by"))
	})
}

// Search is the public forum shape most likely to be built against from outside this
// repo, and its twelve keys are assembled by hand in a loop. PostNumber drives the
// deep-link page, so it is set to something past the first page to prove `page` is
// derived rather than hardcoded.
func TestForumSearchResultMatchesSchema(t *testing.T) {
	result := model.ForumSearchResult{
		PostID: 1, Body: "hello", TopicID: 2, TopicTitle: "A topic",
		ForumID: 3, ForumName: "General", UserID: 4, Username: "author",
		CreatedAt: time.Now(), Snippet: "hel!!MARK_START!!lo!!MARK_END!!",
		PostNumber: 26,
	}

	body := searchResultResponse(result)
	assertSameKeys(t, "ForumSearchResult", body)

	if body["page"] != 2 {
		t.Errorf("page = %v for post number 26 at %d per page, want 2",
			body["page"], topicPerPage)
	}
}

// The edit history is the one forum response that marshals a model directly rather
// than building a map, so the schema is pinned to the struct's own JSON tags.
//
// This assertion is derived from the struct, not from a list. The version that used a
// list is why `reconstruction_failed` was undocumented.
func TestForumPostEditSchemaMatchesTheModel(t *testing.T) {
	documented := schemaProperties(t, "ForumPostEdit")
	onTheWire := wireKeys(t, model.ForumPostEdit{})

	if !reflect.DeepEqual(documented, onTheWire) {
		t.Errorf("schema ForumPostEdit documents %v,\nbut model.ForumPostEdit marshals %v",
			documented, onTheWire)
	}
}

// Known limits of this file, so the next person does not mistake it for more than
// it is:
//
//   - Key sets only. A field changing from int64 to string, losing `nullable`, or
//     losing `format: date-time` passes.
//   - No `required:`. The response schemas mark nothing required, so the generated
//     TypeScript makes every field optional and this test cannot tell an
//     always-present key from a conditional one. That is also why there is no
//     minimal fixture here: asserting the always-present set would mean hardcoding
//     it, which is the mistake at the top of this file.
//   - Maximal fixtures. Every optional field is populated, so a regression that
//     makes a currently-unconditional key conditional stays green — the fixture
//     satisfies the new condition.
//   - Envelopes are not covered: {categories}, {forum, topics, total, page,
//     per_page, can_create_topic}, {topic, posts, ...}, {results, ...}, {topic,
//     post}, {post}, {edits}. Pinning those needs handler-level HTTP tests.
//
// #233 tracks closing the first three together: add `required:`, then assert both a
// maximal and a minimal fixture and check JSON kinds against type/format/nullable.
// That upgrade needs no new dependency — encoding/json plus the schema this file
// already parses is enough.
