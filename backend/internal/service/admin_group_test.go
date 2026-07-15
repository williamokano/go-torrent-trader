package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// fakeGroupStore implements both repository.GroupRepository and
// repository.GroupWriteRepository in memory, returning sql.ErrNoRows for misses
// so the service's not-found mapping is exercised.
type fakeGroupStore struct {
	groups  []*model.Group
	members map[int64]int
	nextID  int64
	deleted []int64
}

func newFakeGroupStore() *fakeGroupStore {
	return &fakeGroupStore{
		groups: []*model.Group{
			{ID: 1, Name: "Administrator", Slug: "administrator", Level: 100, IsAdmin: true},
			{ID: 5, Name: "User", Slug: "user", Level: 20},
			{ID: 7, Name: "VIP", Slug: "vip", Level: 60},
		},
		members: map[int64]int{},
		nextID:  8,
	}
}

func (f *fakeGroupStore) GetByID(_ context.Context, id int64) (*model.Group, error) {
	for _, g := range f.groups {
		if g.ID == id {
			cp := *g
			return &cp, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeGroupStore) List(_ context.Context) ([]model.Group, error) {
	out := make([]model.Group, 0, len(f.groups))
	for _, g := range f.groups {
		out = append(out, *g)
	}
	return out, nil
}

func (f *fakeGroupStore) Create(_ context.Context, g *model.Group) error {
	g.ID = f.nextID
	f.nextID++
	stored := *g
	f.groups = append(f.groups, &stored)
	return nil
}

func (f *fakeGroupStore) Update(_ context.Context, g *model.Group) error {
	for _, e := range f.groups {
		if e.ID == g.ID {
			*e = *g
			return nil
		}
	}
	return sql.ErrNoRows
}

func (f *fakeGroupStore) Delete(_ context.Context, id int64) error {
	for i, e := range f.groups {
		if e.ID == id {
			f.groups = append(f.groups[:i], f.groups[i+1:]...)
			f.deleted = append(f.deleted, id)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (f *fakeGroupStore) CountMembers(_ context.Context, id int64) (int, error) {
	return f.members[id], nil
}

func groupStrPtr(s string) *string { return &s }

func newGroupAdminService(store *fakeGroupStore) *AdminService {
	svc := NewAdminService(newMockUserRepo(), store, event.NewInMemoryBus())
	svc.SetGroupWriter(store)
	return svc
}

func validGroupReq() GroupWriteRequest {
	return GroupWriteRequest{Name: "Uploaders", Slug: "uploaders", Level: 30, CanUpload: true}
}

func TestCreateGroup_Success(t *testing.T) {
	store := newFakeGroupStore()
	svc := newGroupAdminService(store)

	g, err := svc.CreateGroup(context.Background(), validGroupReq())
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if g.ID == 0 {
		t.Fatal("created group has no ID")
	}
	if !g.CanUpload || g.Level != 30 {
		t.Errorf("fields not persisted: %+v", g)
	}
}

func TestCreateGroup_DerivesSlugFromName(t *testing.T) {
	store := newFakeGroupStore()
	svc := newGroupAdminService(store)

	req := GroupWriteRequest{Name: "Power Users!", Level: 40}
	g, err := svc.CreateGroup(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if g.Slug != "power-users" {
		t.Errorf("slug = %q, want power-users", g.Slug)
	}
}

func TestCreateGroup_Validation(t *testing.T) {
	tests := []struct {
		name string
		req  GroupWriteRequest
		want error
	}{
		{"empty name", GroupWriteRequest{Name: "  "}, ErrGroupNameRequired},
		{"bad slug", GroupWriteRequest{Name: "X", Slug: "Bad Slug"}, ErrGroupInvalidSlug},
		{"bad color", GroupWriteRequest{Name: "X", Slug: "x", Color: groupStrPtr("red")}, ErrGroupInvalidColor},
		{"level too high", GroupWriteRequest{Name: "X", Slug: "x", Level: 5000}, ErrGroupInvalidLevel},
		{"negative level", GroupWriteRequest{Name: "X", Slug: "x", Level: -1}, ErrGroupInvalidLevel},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeGroupStore()
			svc := newGroupAdminService(store)
			_, err := svc.CreateGroup(context.Background(), tc.req)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreateGroup_ConflictOnNameAndSlug(t *testing.T) {
	store := newFakeGroupStore()
	svc := newGroupAdminService(store)

	// "administrator" name already exists (case-insensitive).
	_, err := svc.CreateGroup(context.Background(), GroupWriteRequest{Name: "ADMINISTRATOR", Slug: "admins"})
	if !errors.Is(err, ErrGroupNameTaken) {
		t.Errorf("name conflict: err = %v, want ErrGroupNameTaken", err)
	}

	_, err = svc.CreateGroup(context.Background(), GroupWriteRequest{Name: "Fresh", Slug: "vip"})
	if !errors.Is(err, ErrGroupSlugTaken) {
		t.Errorf("slug conflict: err = %v, want ErrGroupSlugTaken", err)
	}
}

func TestUpdateGroup_Success(t *testing.T) {
	store := newFakeGroupStore()
	svc := newGroupAdminService(store)

	req := GroupWriteRequest{Name: "VIP Plus", Slug: "vip", Level: 65, Color: groupStrPtr("#00AAFF"), CanForum: true}
	g, err := svc.UpdateGroup(context.Background(), 7, req)
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if g.Name != "VIP Plus" || g.Level != 65 || g.Color == nil || *g.Color != "#00AAFF" {
		t.Errorf("update not applied: %+v", g)
	}
}

func TestUpdateGroup_NotFound(t *testing.T) {
	store := newFakeGroupStore()
	svc := newGroupAdminService(store)

	_, err := svc.UpdateGroup(context.Background(), 999, validGroupReq())
	if !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("err = %v, want ErrGroupNotFound", err)
	}
}

func TestUpdateGroup_CannotDemoteAdminGroup(t *testing.T) {
	store := newFakeGroupStore()
	svc := newGroupAdminService(store)

	// Attempt to update the admin group (id 1) with is_admin=false.
	req := GroupWriteRequest{Name: "Administrator", Slug: "administrator", Level: 100, IsAdmin: false}
	_, err := svc.UpdateGroup(context.Background(), adminGroupID, req)
	if !errors.Is(err, ErrGroupProtected) {
		t.Errorf("err = %v, want ErrGroupProtected", err)
	}
}

func TestDeleteGroup_ProtectedBuiltins(t *testing.T) {
	store := newFakeGroupStore()
	svc := newGroupAdminService(store)

	for _, id := range []int64{adminGroupID, defaultGroupID} {
		if err := svc.DeleteGroup(context.Background(), id); !errors.Is(err, ErrGroupProtected) {
			t.Errorf("DeleteGroup(%d) = %v, want ErrGroupProtected", id, err)
		}
	}
}

func TestDeleteGroup_BlockedWhenHasMembers(t *testing.T) {
	store := newFakeGroupStore()
	store.members[7] = 3
	svc := newGroupAdminService(store)

	if err := svc.DeleteGroup(context.Background(), 7); !errors.Is(err, ErrGroupHasMembers) {
		t.Errorf("err = %v, want ErrGroupHasMembers", err)
	}
}

func TestDeleteGroup_Success(t *testing.T) {
	store := newFakeGroupStore()
	svc := newGroupAdminService(store)

	if err := svc.DeleteGroup(context.Background(), 7); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != 7 {
		t.Errorf("group 7 was not deleted: %v", store.deleted)
	}
}

func TestDeleteGroup_NotFound(t *testing.T) {
	store := newFakeGroupStore()
	svc := newGroupAdminService(store)

	if err := svc.DeleteGroup(context.Background(), 999); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("err = %v, want ErrGroupNotFound", err)
	}
}

func TestGroupWrites_UnavailableWithoutWriter(t *testing.T) {
	// No SetGroupWriter — the write path must refuse rather than nil-panic.
	svc := NewAdminService(newMockUserRepo(), newFakeGroupStore(), event.NewInMemoryBus())

	if _, err := svc.CreateGroup(context.Background(), validGroupReq()); !errors.Is(err, ErrGroupWritesUnavailable) {
		t.Errorf("CreateGroup err = %v, want ErrGroupWritesUnavailable", err)
	}
	if err := svc.DeleteGroup(context.Background(), 7); !errors.Is(err, ErrGroupWritesUnavailable) {
		t.Errorf("DeleteGroup err = %v, want ErrGroupWritesUnavailable", err)
	}
}
