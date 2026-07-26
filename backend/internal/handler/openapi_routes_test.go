package handler

import (
	"bytes"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/openapi"
)

// An OpenAPI document is only worth anything if it describes the server that is
// actually running. Hand-written specs stop doing that silently: the endpoint
// moves, the document does not, and nobody notices until an integrator does.
// This repo had three such lies in it when these tests were written — a profile
// endpoint documented by id when the router matched by username, and two report
// endpoints documented on /api/v1/reports when the router had only ever
// registered them under /api/v1/admin.
//
// So the spec is pinned to the router in both directions, the way
// connector/templatefields_test.go pins its help text to the struct it
// describes: nothing registered may go undocumented without being written down
// as debt, and nothing documented may describe a route that does not exist.
//
// The second thing these tests defend is the public/full split. Which
// operations are published is decided per operation by x-audience, and the
// public document is generated rather than maintained — see internal/openapi.
// A path-prefix rule would be simpler and would leak: the six forum topic
// moderation endpoints are staff-only and sit on ordinary member paths.

const (
	fullSpecPath   = "../../api/openapi.yaml"
	publicSpecPath = "../../api/openapi.public.yaml"
)

// minimumRoutes guards the guard. Almost every test in this file is a loop over
// the router's route table, and router.go drops a route whenever the service
// behind it is nil — so an under-wired fullRouterDeps would make all of them
// pass by finding nothing to check. 150 is comfortably under the real count and
// comfortably over what a partially wired router produces.
const minimumRoutes = 150

// normalizePath renders a chi pattern the way the spec writes it.
//
// chi reports `r.Route("/stats", …)` plus `r.Get("/", …)` as the pattern
// "/api/v1/stats/", trailing slash and all, even though the route answers
// requests to "/api/v1/stats". Twelve route groups are built that way, so
// without this every one of them would look undocumented.
func normalizePath(pattern string) string {
	if len(pattern) > 1 {
		return strings.TrimSuffix(pattern, "/")
	}
	return pattern
}

// routeKey is the "METHOD /path" form both the ledger and the spec comparison
// use.
func routeKey(r route) string {
	return r.method + " " + normalizePath(r.pattern)
}

// registeredRoutes walks the fully wired router and returns every route it
// registers, keyed by routeKey.
func registeredRoutes(t *testing.T) map[string]route {
	t.Helper()

	routes := walkRoutes(t, NewRouter(fullRouterDeps(t)))
	if len(routes) < minimumRoutes {
		t.Fatalf("only %d routes found — the router is under-wired and this test proves nothing", len(routes))
	}

	byKey := make(map[string]route, len(routes))
	for _, r := range routes {
		byKey[routeKey(r)] = r
	}
	return byKey
}

// specOperations parses a spec and keys its operations by "METHOD /path".
func specOperations(t *testing.T, path string) map[string]openapi.Operation {
	t.Helper()

	ops, err := openapi.Operations(readSpec(t, path))
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	byKey := make(map[string]openapi.Operation, len(ops))
	for _, op := range ops {
		if existing, dup := byKey[op.Key()]; dup {
			t.Errorf("%s documents %s twice (%s and %s)", path, op.Key(), existing.OperationID, op.OperationID)
		}
		byKey[op.Key()] = op
	}
	return byKey
}

func readSpec(t *testing.T, path string) []byte {
	t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return src
}

// sortedKeys makes failure output stable and diffable.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- spec vs router ---------------------------------------------------------

// Every route the router registers is either documented or written down as
// known debt. A new endpoint that is neither fails here, at the moment its
// author still knows what it does.
func TestEveryRouteIsDocumentedOrKnownUndocumented(t *testing.T) {
	documented := specOperations(t, fullSpecPath)
	routes := registeredRoutes(t)

	var undocumented int
	for _, key := range sortedKeys(routes) {
		if _, ok := documented[key]; ok {
			continue
		}
		if _, known := undocumentedRoutes[key]; known {
			undocumented++
			continue
		}
		t.Errorf("%s is registered but not documented in %s.\n"+
			"Document it (with an x-audience marker), or — only if it genuinely cannot be "+
			"documented now — add %q to undocumentedRoutes in openapi_undocumented.go and say why.",
			key, fullSpecPath, key)
	}

	t.Logf("%d routes: %d documented, %d in the debt ledger", len(routes), len(routes)-undocumented, undocumented)
}

// The other direction. A documented operation that matches no route is the
// failure mode this whole file exists for: the document keeps promising an
// endpoint the server does not serve, and only an integrator finds out.
func TestNoDocumentedOperationIsPhantom(t *testing.T) {
	documented := specOperations(t, fullSpecPath)
	routes := registeredRoutes(t)

	for _, key := range sortedKeys(documented) {
		if _, ok := routes[key]; !ok {
			t.Errorf("%s documents %s (operationId %s), but no such route is registered",
				fullSpecPath, key, documented[key].OperationID)
		}
	}
}

// The ledger has to shrink honestly. An entry naming a route that was deleted,
// or one that has since been documented, would sit there forever making the
// debt look larger than it is — and would hide the fact that the endpoint it
// names is gone. Mirrors TestNoDescriptionOutlivesItsField in
// internal/connector.
func TestNoStaleUndocumentedEntries(t *testing.T) {
	documented := specOperations(t, fullSpecPath)
	routes := registeredRoutes(t)

	for _, key := range sortedKeys(undocumentedRoutes) {
		if _, ok := routes[key]; !ok {
			t.Errorf("undocumentedRoutes lists %q, which is not a registered route — remove it", key)
			continue
		}
		if op, ok := documented[key]; ok {
			t.Errorf("undocumentedRoutes lists %q, but %s documents it as %s — remove it from the ledger",
				key, fullSpecPath, op.OperationID)
		}
	}
}

// Every operation must declare an audience. Without one the generator would
// treat it as public by omission, which is the wrong default for a file that
// also describes the admin surface.
func TestEveryDocumentedOperationDeclaresAnAudience(t *testing.T) {
	documented := specOperations(t, fullSpecPath)
	if len(documented) == 0 {
		t.Fatalf("%s documents no operations", fullSpecPath)
	}

	for _, key := range sortedKeys(documented) {
		switch documented[key].Audience {
		case openapi.AudiencePublic, openapi.AudienceInternal:
		case "":
			t.Errorf("%s (operationId %s) has no %s — every operation must declare one",
				key, documented[key].OperationID, openapi.AudienceKey)
		default:
			t.Errorf("%s (operationId %s) has %s: %q — must be %q or %q",
				key, documented[key].OperationID, openapi.AudienceKey,
				documented[key].Audience, openapi.AudiencePublic, openapi.AudienceInternal)
		}
	}
}

// --- what must never be published -------------------------------------------

// Everything under /api/v1/admin is behind RequireAdmin or RequireStaff
// (route_authz_test.go proves that separately). None of it belongs in the
// published document.
func TestEveryAdminRouteIsInternal(t *testing.T) {
	documented := specOperations(t, fullSpecPath)
	public := specOperations(t, publicSpecPath)

	var checked int
	for _, key := range sortedKeys(registeredRoutes(t)) {
		if !strings.Contains(key, " /api/v1/admin") {
			continue
		}
		checked++

		if op, ok := documented[key]; ok && op.Audience != openapi.AudienceInternal {
			t.Errorf("%s is an admin route documented as %s: %q — it must be %q",
				key, openapi.AudienceKey, op.Audience, openapi.AudienceInternal)
		}
		if _, ok := public[key]; ok {
			t.Errorf("%s is an admin route and it appears in %s", key, publicSpecPath)
		}
	}

	if checked < 20 {
		t.Fatalf("only %d admin routes found — the router is under-wired and this test proves nothing", checked)
	}
	t.Logf("checked %d admin routes", checked)
}

// These six are the reason x-audience is per-operation rather than derived from
// the path. They are forum topic moderation: staff-only, but registered on
// ordinary /api/v1/forums/... paths with no route-level guard, because the
// service layer does the authorizing (see the "Moderation endpoints (staff only
// enforced at service layer)" group in router.go).
//
// A filter that published everything outside /api/v1/admin would publish every
// one of them, and the document would tell any member that they can rename,
// move, lock and pin other people's topics. Hard-coded rather than derived: the
// point is that nothing about the route itself reveals the constraint.
func TestServiceLayerStaffRoutesAreInternal(t *testing.T) {
	staffOnly := []string{
		"POST /api/v1/forums/topics/{id}/lock",
		"POST /api/v1/forums/topics/{id}/unlock",
		"POST /api/v1/forums/topics/{id}/pin",
		"POST /api/v1/forums/topics/{id}/unpin",
		"POST /api/v1/forums/topics/{id}/move",
		"PUT /api/v1/forums/topics/{id}/title",
	}

	documented := specOperations(t, fullSpecPath)
	public := specOperations(t, publicSpecPath)
	routes := registeredRoutes(t)

	for _, key := range staffOnly {
		// If one of these is ever renamed or removed, this list has to be
		// revisited — silently checking a route that no longer exists would
		// leave the real one unguarded.
		if _, ok := routes[key]; !ok {
			t.Errorf("%q is no longer a registered route — update this list to match router.go", key)
			continue
		}
		if op, ok := documented[key]; ok && op.Audience != openapi.AudienceInternal {
			t.Errorf("%s is staff-only (enforced in the service layer) but documented as %s: %q",
				key, openapi.AudienceKey, op.Audience)
		}
		if _, ok := public[key]; ok {
			t.Errorf("%s is staff-only (enforced in the service layer) but appears in %s",
				key, publicSpecPath)
		}
	}
}

// --- the generated document -------------------------------------------------

// The checked-in public document must be exactly what the generator produces
// from the full one. This is the drift check, and it lives in the test suite
// rather than in a CI step so that it cannot be skipped by a workflow edit.
func TestPublicSpecIsUpToDate(t *testing.T) {
	generated, err := openapi.Public(readSpec(t, fullSpecPath))
	if err != nil {
		t.Fatalf("generating the public spec: %v", err)
	}

	if !bytes.Equal(generated, readSpec(t, publicSpecPath)) {
		t.Errorf("%s is out of date with %s — run: task generate:openapi", publicSpecPath, fullSpecPath)
	}
}

// The public document may only ever be a subset of the full one. Anything in it
// that the full spec does not mark public got there some other way, and the
// full spec has stopped being the source of truth.
func TestPublicSpecIsSubsetOfFull(t *testing.T) {
	full := specOperations(t, fullSpecPath)
	public := specOperations(t, publicSpecPath)

	if len(public) == 0 {
		t.Fatalf("%s documents no operations", publicSpecPath)
	}

	for _, key := range sortedKeys(public) {
		op, ok := full[key]
		if !ok {
			t.Errorf("%s documents %s, which %s does not", publicSpecPath, key, fullSpecPath)
			continue
		}
		if op.Audience != openapi.AudiencePublic {
			t.Errorf("%s documents %s, which %s marks %s: %q",
				publicSpecPath, key, fullSpecPath, openapi.AudienceKey, op.Audience)
		}
	}

	// And the complement: every public operation in the full spec has to be
	// present, or the generator is dropping things it was not asked to.
	for _, key := range sortedKeys(full) {
		if full[key].Audience != openapi.AudiencePublic {
			continue
		}
		if _, ok := public[key]; !ok {
			t.Errorf("%s marks %s public, but it is missing from %s", fullSpecPath, key, publicSpecPath)
		}
	}
}

// x-audience is an authoring concern. Publishing it would tell a reader that a
// filtered document exists and invite them to guess at what was filtered out.
func TestPublicSpecCarriesNoAuthoringMarkers(t *testing.T) {
	src := readSpec(t, publicSpecPath)

	if bytes.Contains(src, []byte(openapi.AudienceKey)) {
		t.Errorf("%s still contains %s markers", publicSpecPath, openapi.AudienceKey)
	}
	if bytes.Contains(src, []byte("/api/v1/admin")) {
		t.Errorf("%s mentions /api/v1/admin", publicSpecPath)
	}
}
