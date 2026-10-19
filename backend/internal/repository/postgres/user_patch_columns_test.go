package postgres

import (
	"reflect"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// Every UserPatch field must reach the SQL. A field added to the patch but
// not to userPatchSets would be accepted, applied in memory by Apply, and
// silently never written.
func TestUserPatchSetsCoverEveryField(t *testing.T) {
	var p model.UserPatch
	p.SetEnabled(true, time.Now()) // enabled, EndConfirmation, DisabledUntil, ActivateAt

	v := reflect.ValueOf(&p).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() || f.Kind() != reflect.Struct {
			continue
		}
		f.FieldByName("Valid").SetBool(true)
	}

	sets, _ := userPatchSets(p)
	if want := v.NumField(); len(sets) != want {
		t.Fatalf("userPatchSets wrote %d columns for a patch with all %d fields set: %v", len(sets), want, sets)
	}
}
