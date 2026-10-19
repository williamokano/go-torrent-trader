package model

import (
	"reflect"
	"testing"
	"time"
)

func TestUserApplyPatch(t *testing.T) {
	earlier := time.Now().Add(-time.Hour)
	u := User{Username: "a", Enabled: false, AwaitingConfirmation: true, Title: ptrTo("t")}

	u.Apply(EnablePatch(time.Now()))
	if !u.Enabled || u.AwaitingConfirmation || u.ActivatedAt == nil {
		t.Fatalf("after enable: enabled=%v awaiting=%v activated=%v", u.Enabled, u.AwaitingConfirmation, u.ActivatedAt)
	}
	stamped := *u.ActivatedAt

	u.Apply(UserPatch{ActivateAt: To(earlier), Title: To[*string](nil)})
	if !u.ActivatedAt.Equal(stamped) {
		t.Error("ActivateAt overwrote an existing activation")
	}
	if u.Title != nil {
		t.Error("a patch could not clear a nullable column")
	}
	if u.Username != "a" {
		t.Error("an unpatched field changed")
	}

	u.Apply(DisablePatch())
	if u.Enabled || u.AwaitingConfirmation {
		t.Errorf("after disable: enabled=%v awaiting=%v", u.Enabled, u.AwaitingConfirmation)
	}
}

func ptrTo[T any](v T) *T { return &v }

// Every UserPatch field must change the user in Apply, so the in-memory copy
// a caller keeps matches what was written.
func TestApplyCoversEveryPatchField(t *testing.T) {
	samples := map[reflect.Type]reflect.Value{
		reflect.TypeOf(""):                reflect.ValueOf("x"),
		reflect.TypeOf(int64(0)):          reflect.ValueOf(int64(7)),
		reflect.TypeOf(true):              reflect.ValueOf(true),
		reflect.TypeOf((*string)(nil)):    reflect.ValueOf(ptrTo("x")),
		reflect.TypeOf((*time.Time)(nil)): reflect.ValueOf(ptrTo(time.Now())),
		reflect.TypeOf(time.Time{}):       reflect.ValueOf(time.Now()),
	}
	pt := reflect.TypeOf(UserPatch{})
	for i := 0; i < pt.NumField(); i++ {
		field := pt.Field(i)
		if !field.IsExported() {
			continue // enabled: covered through SetEnabled above
		}
		var p UserPatch
		fv := reflect.ValueOf(&p).Elem().Field(i)
		if fv.Kind() == reflect.Bool {
			fv.SetBool(true) // EndConfirmation
		} else {
			sample, ok := samples[fv.FieldByName("Value").Type()]
			if !ok {
				t.Fatalf("no sample value for %s", field.Name)
			}
			fv.FieldByName("Value").Set(sample)
			fv.FieldByName("Valid").SetBool(true)
		}

		u := User{AwaitingConfirmation: true}
		before := u
		u.Apply(p)
		if reflect.DeepEqual(u, before) {
			t.Errorf("Apply ignores UserPatch.%s", field.Name)
		}
	}
}
