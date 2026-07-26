package openapi

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// spec assembles a small but structurally complete document. Tests vary one
// thing at a time against it rather than each carrying a wall of YAML.
const spec = `# authoring header that must not survive
openapi: 3.0.3
info:
  title: Example API (Full)
  version: 1.0.0
  description: the internal one
paths:
  /public:
    get:
      operationId: getPublic
      x-audience: public
      security:
        - bearerAuth: []
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Public'
  /mixed:
    parameters:
      - name: id
        in: query
    get:
      operationId: getMixed
      x-audience: public
      responses:
        '200':
          description: ok
    delete:
      operationId: deleteMixed
      x-audience: internal
      responses:
        '204':
          description: gone
  /admin:
    get:
      operationId: getAdmin
      x-audience: internal
      security:
        - adminAuth: []
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AdminOnly'
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
    adminAuth:
      type: http
      scheme: bearer
    unusedAuth:
      type: http
      scheme: bearer
  schemas:
    Public:
      type: object
      properties:
        nested:
          $ref: '#/components/schemas/Nested'
    Nested:
      type: object
      properties:
        deep:
          $ref: '#/components/schemas/Deep'
    Deep:
      type: string
    AdminOnly:
      type: object
      properties:
        secret:
          $ref: '#/components/schemas/AdminDetail'
    AdminDetail:
      type: string
    Orphan:
      type: string
`

// publicOf runs the generator and fails the test on error.
func publicOf(t *testing.T, src string) []byte {
	t.Helper()

	out, err := Public([]byte(src))
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	return out
}

// decode parses generated YAML back into plain maps for assertions that do not
// care about formatting.
func decode(t *testing.T, src []byte) map[string]any {
	t.Helper()

	var out map[string]any
	if err := yaml.Unmarshal(src, &out); err != nil {
		t.Fatalf("generated spec is not valid YAML: %v\n%s", err, src)
	}
	return out
}

func paths(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()

	p, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("generated spec has no paths mapping")
	}
	return p
}

func section(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()

	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("generated spec has no components mapping")
	}
	s, ok := components[name].(map[string]any)
	if !ok {
		t.Fatalf("generated spec has no components.%s mapping", name)
	}
	return s
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- Operations -------------------------------------------------------------

func TestOperationsListsEveryOperationWithItsAudience(t *testing.T) {
	ops, err := Operations([]byte(spec))
	if err != nil {
		t.Fatalf("Operations: %v", err)
	}

	want := []Operation{
		{Path: "/admin", Method: "GET", Audience: AudienceInternal, OperationID: "getAdmin"},
		{Path: "/mixed", Method: "DELETE", Audience: AudienceInternal, OperationID: "deleteMixed"},
		{Path: "/mixed", Method: "GET", Audience: AudiencePublic, OperationID: "getMixed"},
		{Path: "/public", Method: "GET", Audience: AudiencePublic, OperationID: "getPublic"},
	}
	if len(ops) != len(want) {
		t.Fatalf("Operations returned %d operations, want %d: %+v", len(ops), len(want), ops)
	}
	// Sorted by path then method, so the comparison is positional on purpose:
	// the ordering is part of what makes failure output diffable.
	for i := range want {
		if ops[i] != want[i] {
			t.Errorf("operation %d = %+v, want %+v", i, ops[i], want[i])
		}
	}
}

func TestOperationsIgnoresPathItemFieldsThatAreNotOperations(t *testing.T) {
	// /mixed carries a path-level `parameters` list. Treating it as an
	// operation would invent a "PARAMETERS /mixed" route.
	ops, err := Operations([]byte(spec))
	if err != nil {
		t.Fatalf("Operations: %v", err)
	}
	for _, op := range ops {
		if op.Method == "PARAMETERS" {
			t.Errorf("path-level parameters were read as an operation: %+v", op)
		}
	}
}

func TestOperationKey(t *testing.T) {
	got := Operation{Path: "/api/v1/torrents/{id}", Method: "GET"}.Key()
	if want := "GET /api/v1/torrents/{id}"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}

func TestOperationsReportsAMissingAudienceAsEmpty(t *testing.T) {
	ops, err := Operations([]byte(`
openapi: 3.0.3
info: {title: x, description: y}
paths:
  /a:
    get:
      operationId: a
`))
	if err != nil {
		t.Fatalf("Operations: %v", err)
	}
	if len(ops) != 1 || ops[0].Audience != "" {
		t.Errorf("got %+v, want a single operation with an empty audience", ops)
	}
}

// --- Public: what is dropped ------------------------------------------------

func TestPublicDropsInternalOperations(t *testing.T) {
	doc := decode(t, publicOf(t, spec))
	p := paths(t, doc)

	mixed, ok := p["/mixed"].(map[string]any)
	if !ok {
		t.Fatalf("/mixed is missing from the public spec: %v", keys(p))
	}
	if _, still := mixed["delete"]; still {
		t.Errorf("/mixed kept its internal delete operation")
	}
	if _, kept := mixed["get"]; !kept {
		t.Errorf("/mixed lost its public get operation")
	}
}

func TestPublicDropsPathItemsLeftWithNoOperations(t *testing.T) {
	doc := decode(t, publicOf(t, spec))

	if _, still := paths(t, doc)["/admin"]; still {
		t.Errorf("/admin survived even though its only operation was internal")
	}
}

func TestPublicKeepsPathItemFieldsAlongsideTheSurvivingOperations(t *testing.T) {
	doc := decode(t, publicOf(t, spec))

	mixed := paths(t, doc)["/mixed"].(map[string]any)
	if _, ok := mixed["parameters"]; !ok {
		t.Errorf("/mixed lost its path-level parameters")
	}
}

func TestPublicStripsAudienceMarkers(t *testing.T) {
	out := publicOf(t, spec)

	if strings.Contains(string(out), AudienceKey) {
		t.Errorf("generated spec still contains %s:\n%s", AudienceKey, out)
	}
}

// --- Public: component pruning ----------------------------------------------

func TestPublicPrunesSchemasReachableOnlyFromDroppedOperations(t *testing.T) {
	schemas := section(t, decode(t, publicOf(t, spec)), "schemas")

	for _, name := range []string{"AdminOnly", "AdminDetail"} {
		if _, still := schemas[name]; still {
			t.Errorf("%s is only referenced by a dropped operation but survived", name)
		}
	}
}

// The transitive case is the one a naive implementation gets wrong: Deep is
// referenced by Nested, which is referenced by Public, which is referenced by a
// surviving operation. Dropping it would leave a $ref pointing at nothing.
func TestPublicKeepsTransitivelyReferencedSchemas(t *testing.T) {
	schemas := section(t, decode(t, publicOf(t, spec)), "schemas")

	for _, name := range []string{"Public", "Nested", "Deep"} {
		if _, kept := schemas[name]; !kept {
			t.Errorf("%s is reachable from a surviving operation but was pruned (kept: %v)",
				name, keys(schemas))
		}
	}
}

func TestPublicPrunesSchemasNothingReferences(t *testing.T) {
	schemas := section(t, decode(t, publicOf(t, spec)), "schemas")

	if _, still := schemas["Orphan"]; still {
		t.Errorf("Orphan is referenced by nothing at all but survived")
	}
}

func TestPublicKeepsSchemasReferencedFromOtherComponentSections(t *testing.T) {
	src := `
openapi: 3.0.3
info: {title: t, description: d}
paths:
  /a:
    get:
      operationId: a
      x-audience: public
      responses:
        '500':
          $ref: '#/components/responses/Failure'
components:
  responses:
    Failure:
      description: boom
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
  schemas:
    Error:
      type: object
    Orphan:
      type: string
`
	schemas := section(t, decode(t, publicOf(t, src)), "schemas")

	if _, kept := schemas["Error"]; !kept {
		t.Errorf("Error is referenced from components.responses but was pruned")
	}
	if _, still := schemas["Orphan"]; still {
		t.Errorf("Orphan survived")
	}
}

func TestPublicPrunesUnusedSecuritySchemes(t *testing.T) {
	schemes := section(t, decode(t, publicOf(t, spec)), "securitySchemes")

	if _, kept := schemes["bearerAuth"]; !kept {
		t.Errorf("bearerAuth is used by a surviving operation but was pruned")
	}
	for _, name := range []string{"adminAuth", "unusedAuth"} {
		if _, still := schemes[name]; still {
			t.Errorf("%s is referenced by nothing surviving but was kept", name)
		}
	}
}

// A root-level `security` applies to every operation, so a scheme named only
// there is still in use.
func TestPublicKeepsSecuritySchemesNamedByTheRootRequirement(t *testing.T) {
	src := `
openapi: 3.0.3
info: {title: t, description: d}
security:
  - globalAuth: []
paths:
  /a:
    get:
      operationId: a
      x-audience: public
      responses:
        '200':
          description: ok
components:
  securitySchemes:
    globalAuth:
      type: http
      scheme: bearer
    unusedAuth:
      type: http
      scheme: bearer
`
	schemes := section(t, decode(t, publicOf(t, src)), "securitySchemes")

	if _, kept := schemes["globalAuth"]; !kept {
		t.Errorf("globalAuth is named by the root security requirement but was pruned")
	}
	if _, still := schemes["unusedAuth"]; still {
		t.Errorf("unusedAuth survived")
	}
}

func TestPublicRemovesComponentSectionsItEmpties(t *testing.T) {
	src := `
openapi: 3.0.3
info: {title: t, description: d}
paths:
  /a:
    get:
      operationId: a
      x-audience: public
      responses:
        '200':
          description: ok
components:
  schemas:
    Orphan:
      type: string
  securitySchemes:
    unusedAuth:
      type: http
      scheme: bearer
`
	doc := decode(t, publicOf(t, src))

	// `components: {}` would read as "this API has no shared components",
	// which is a claim; leaving the key out makes none.
	if _, still := doc["components"]; still {
		t.Errorf("components survived with nothing in it: %v", doc["components"])
	}
}

func TestPublicToleratesASpecWithNoComponents(t *testing.T) {
	src := `
openapi: 3.0.3
info: {title: t, description: d}
paths:
  /a:
    get:
      operationId: a
      x-audience: public
      responses:
        '200':
          description: ok
`
	doc := decode(t, publicOf(t, src))

	if len(paths(t, doc)) != 1 {
		t.Errorf("paths = %v, want just /a", keys(paths(t, doc)))
	}
}

// --- Public: the document itself --------------------------------------------

func TestPublicRewritesTitleAndDescription(t *testing.T) {
	doc := decode(t, publicOf(t, spec))

	info, ok := doc["info"].(map[string]any)
	if !ok {
		t.Fatalf("generated spec has no info mapping")
	}
	if info["title"] != publicTitle {
		t.Errorf("title = %v, want %q", info["title"], publicTitle)
	}
	description, _ := info["description"].(string)
	if description == "the internal one" || !strings.Contains(description, "deliberately excluded") {
		t.Errorf("description was not replaced with the public-facing one: %q", description)
	}
	if info["version"] != "1.0.0" {
		t.Errorf("version = %v, want it left alone", info["version"])
	}
}

func TestPublicEmitsAGeneratedHeaderAndDropsTheAuthoringOne(t *testing.T) {
	out := string(publicOf(t, spec))

	if !strings.HasPrefix(out, "# Code generated by cmd/openapi-public. DO NOT EDIT.\n") {
		t.Errorf("generated spec does not start with the DO NOT EDIT header:\n%s", firstLines(out, 3))
	}
	if !strings.Contains(out, "# source: backend/api/openapi.yaml") {
		t.Errorf("generated spec does not name its source")
	}
	if strings.Contains(out, "authoring header that must not survive") {
		t.Errorf("the full spec's own header leaked into the generated file")
	}
}

// Byte-for-byte reproducibility is what makes the checked-in public spec
// verifiable: TestPublicSpecIsUpToDate compares bytes, so anything
// order-dependent here would produce a test that fails at random.
func TestPublicIsDeterministic(t *testing.T) {
	first := publicOf(t, spec)
	for i := range 20 {
		if got := publicOf(t, spec); string(got) != string(first) {
			t.Fatalf("run %d differed from the first:\n%s\n---\n%s", i, first, got)
		}
	}
}

// Round-tripping through map[string]any would sort every mapping
// alphabetically, turning a one-line change into an unreadable diff. Key order
// coming from the source is the evidence that did not happen.
func TestPublicPreservesKeyOrder(t *testing.T) {
	out := string(publicOf(t, spec))

	openapiAt := strings.Index(out, "openapi:")
	infoAt := strings.Index(out, "info:")
	pathsAt := strings.Index(out, "paths:")
	if openapiAt >= infoAt || infoAt >= pathsAt {
		t.Errorf("root keys were reordered (openapi=%d info=%d paths=%d)", openapiAt, infoAt, pathsAt)
	}
	if publicAt, mixedAt := strings.Index(out, "/public:"), strings.Index(out, "/mixed:"); publicAt > mixedAt {
		t.Errorf("path items were reordered (/public=%d /mixed=%d)", publicAt, mixedAt)
	}
}

func TestPublicPreservesComments(t *testing.T) {
	src := `
openapi: 3.0.3
info: {title: t, description: d}
paths:
  /a:
    get:
      operationId: a
      x-audience: public
      # this note explains something non-obvious
      responses:
        '200':
          description: ok
`
	if !strings.Contains(string(publicOf(t, src)), "# this note explains something non-obvious") {
		t.Errorf("a comment in the source spec was dropped")
	}
}

// --- error handling ---------------------------------------------------------

func TestPublicRejectsMalformedSpecs(t *testing.T) {
	for name, src := range map[string]string{
		"not YAML":              "\t: : :\n  - [",
		"empty":                 "",
		"root is a sequence":    "- a\n- b\n",
		"no paths":              "openapi: 3.0.3\ninfo: {title: t, description: d}\n",
		"paths is a sequence":   "openapi: 3.0.3\ninfo: {title: t, description: d}\npaths:\n  - /a\n",
		"path item is a scalar": "openapi: 3.0.3\ninfo: {title: t, description: d}\npaths:\n  /a: nope\n",
		"no info":               "openapi: 3.0.3\npaths:\n  /a:\n    get:\n      operationId: a\n      x-audience: public\n",
		"info has no title":     "openapi: 3.0.3\ninfo: {description: d}\npaths:\n  /a:\n    get: {operationId: a, x-audience: public}\n",
		"info has no description": "openapi: 3.0.3\ninfo: {title: t}\npaths:\n  /a:\n    get: " +
			"{operationId: a, x-audience: public}\n",
		"components is a scalar": "openapi: 3.0.3\ninfo: {title: t, description: d}\npaths:\n  /a:\n    get: " +
			"{operationId: a, x-audience: public}\ncomponents: nope\n",
		"schemas is a sequence": "openapi: 3.0.3\ninfo: {title: t, description: d}\npaths:\n  /a:\n    get: " +
			"{operationId: a, x-audience: public}\ncomponents:\n  schemas:\n    - A\n",
		"securitySchemes is a sequence": "openapi: 3.0.3\ninfo: {title: t, description: d}\npaths:\n  /a:\n    get: " +
			"{operationId: a, x-audience: public}\ncomponents:\n  securitySchemes:\n    - A\n",
	} {
		t.Run(name, func(t *testing.T) {
			if out, err := Public([]byte(src)); err == nil {
				t.Errorf("Public accepted a spec that is %s:\n%s", name, out)
			}
		})
	}
}

func TestOperationsRejectsMalformedSpecs(t *testing.T) {
	for name, src := range map[string]string{
		"not YAML":              "\t: : :\n  - [",
		"no paths":              "openapi: 3.0.3\n",
		"path item is a scalar": "openapi: 3.0.3\npaths:\n  /a: nope\n",
	} {
		t.Run(name, func(t *testing.T) {
			if ops, err := Operations([]byte(src)); err == nil {
				t.Errorf("Operations accepted a spec that is %s: %+v", name, ops)
			}
		})
	}
}

func TestPublicRejectsMultipleDocuments(t *testing.T) {
	src := "openapi: 3.0.3\ninfo: {title: t, description: d}\npaths: {}\n---\nopenapi: 3.0.3\n"

	if _, err := Public([]byte(src)); err == nil {
		t.Errorf("Public accepted a multi-document YAML stream")
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
