package handler

import (
	"reflect"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// #233, limit 4. The maximal fixtures are hand-built, so they are maximal only with
// respect to the fields that existed on the day they were written. A field added to
// the model later is left at its zero value, so a response key guarded by
// `if x != nil` never fires, and the key-set guard — which can only compare what was
// actually emitted — reports nothing.
//
// Demonstrated during #238's review: adding a nil-guarded `resp["last_post_id"]` to
// forumResponse left both suites green. Adding a field is the common change, so this
// is the likelier direction of drift, and it was the one nothing watched.
//
// fillMaximal populates every field by reflection, so the fixture grows with the
// model instead of being remembered. A new pointer field is allocated, so its
// conditional key fires, so the key-set guard sees it and demands a schema entry.
func fillMaximal(t *testing.T, target interface{}) {
	t.Helper()

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		t.Fatalf("fillMaximal wants a pointer to a struct, got %T", target)
	}
	fillStruct(t, v.Elem(), 0)
}

// fillStruct assigns a distinguishable non-zero value to every settable field.
//
// Values are derived from the field's position rather than being constants, so a
// test comparing two fields cannot pass by their happening to match.
func fillStruct(t *testing.T, v reflect.Value, depth int) {
	t.Helper()
	if depth > 4 {
		return // a self-referencing model would otherwise recurse forever
	}

	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue // unexported
		}
		fillValue(t, f, i+1, depth)
	}
}

func fillValue(t *testing.T, f reflect.Value, seed, depth int) {
	t.Helper()

	// time.Time is a struct, but filling it field by field produces nonsense.
	if f.Type() == reflect.TypeOf(time.Time{}) {
		f.Set(reflect.ValueOf(time.Now().UTC().Truncate(time.Second)))
		return
	}

	switch f.Kind() {
	case reflect.String:
		f.SetString("filled")
	case reflect.Bool:
		f.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.SetInt(int64(seed))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		f.SetUint(uint64(seed))
	case reflect.Float32, reflect.Float64:
		f.SetFloat(float64(seed))
	case reflect.Pointer:
		// The case this exists for: an unset pointer is what silences a
		// conditional key.
		f.Set(reflect.New(f.Type().Elem()))
		fillValue(t, f.Elem(), seed, depth+1)
	case reflect.Slice:
		elem := reflect.New(f.Type().Elem()).Elem()
		fillValue(t, elem, seed, depth+1)
		f.Set(reflect.Append(reflect.MakeSlice(f.Type(), 0, 1), elem))
	case reflect.Struct:
		fillStruct(t, f, depth+1)
	}
}

// Every response builder, driven by a fixture that is maximal by construction. A
// field added to any of these models makes its conditional key fire here, and the
// key-set assertion then demands a schema entry for it.
//
// The staff viewer is used deliberately: it is the branch that emits the most keys,
// so it exercises the largest surface.
func TestReflectionBuiltFixturesStayPinnedToTheSchemas(t *testing.T) {
	var forum model.Forum
	fillMaximal(t, &forum)
	assertSameKeys(t, "Forum", forumResponse(&forum))

	var topic model.ForumTopic
	fillMaximal(t, &topic)
	assertSameKeys(t, "ForumTopic", topicResponse(&topic))

	var post model.ForumPost
	fillMaximal(t, &post)
	assertSameKeys(t, "ForumPost", postResponse(&post, true))

	var result model.ForumSearchResult
	fillMaximal(t, &result)
	assertSameKeys(t, "ForumSearchResult", searchResultResponse(result))
}

// The filler is only worth anything if it genuinely leaves nothing zero, so that is
// asserted rather than assumed — a filler with a gap would restore the exact blind
// spot it was written to close, and silently.
func TestFillMaximalLeavesNoFieldZero(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target interface{}
	}{
		{name: "Forum", target: &model.Forum{}},
		{name: "ForumTopic", target: &model.ForumTopic{}},
		{name: "ForumPost", target: &model.ForumPost{}},
		{name: "ForumSearchResult", target: &model.ForumSearchResult{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fillMaximal(t, tc.target)

			v := reflect.ValueOf(tc.target).Elem()
			for i := 0; i < v.NumField(); i++ {
				f := v.Field(i)
				if !f.CanSet() {
					continue
				}
				if f.IsZero() {
					t.Errorf("%s.%s is still zero after fillMaximal, so a response key "+
						"guarded on it would never fire and could go undocumented",
						tc.name, v.Type().Field(i).Name)
				}
			}
		})
	}
}
