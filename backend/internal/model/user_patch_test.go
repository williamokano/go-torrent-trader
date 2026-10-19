package model

import (
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
