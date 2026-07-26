package mapping

import (
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
)

func corpusSchema() schema.Schema {
	return schema.Schema{Database: "torrenttrader", Tables: []schema.Table{
		{Name: "users", Columns: []schema.Column{
			{Name: "id", Type: "int unsigned"},
			{Name: "username", Type: "varchar(40)"},
			{Name: "passkey", Type: "varchar(32)"},
			{Name: "seedbonus", Type: "float"},
		}},
		{Name: "sqlerr", Columns: []schema.Column{{Name: "id", Type: "int"}}},
	}}
}

// The file exists to be edited by hand and read back. An earlier version could
// only write it, which meant nothing proved an edited file was still usable.
func TestRoundTrip(t *testing.T) {
	original := Build(corpusSchema(), "db:3306/torrenttrader")

	rendered, err := Render(original)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	loaded, err := Load(strings.NewReader(string(rendered)))
	if err != nil {
		t.Fatalf("Load: %v\n%s", err, rendered)
	}

	if loaded.Version != original.Version {
		t.Errorf("version = %d, want %d", loaded.Version, original.Version)
	}
	if loaded.Database != original.Database {
		t.Errorf("database = %q, want %q", loaded.Database, original.Database)
	}
	if loaded.Server != original.Server {
		t.Errorf("server = %q, want %q", loaded.Server, original.Server)
	}
	if len(loaded.Tables) != len(original.Tables) {
		t.Fatalf("%d tables, want %d", len(loaded.Tables), len(original.Tables))
	}

	for i, want := range original.Tables {
		got := loaded.Tables[i]
		if got.Name != want.Name {
			t.Errorf("table %d is %q, want %q — order is not preserved", i, got.Name, want.Name)
		}
		if got.Action != want.Action || got.Target != want.Target {
			t.Errorf("%s: %q -> %q, want %q -> %q", want.Name, got.Action, got.Target, want.Action, want.Target)
		}
		if len(got.Columns) != len(want.Columns) {
			t.Errorf("%s: %d columns, want %d", want.Name, len(got.Columns), len(want.Columns))
			continue
		}
		for j, wantCol := range want.Columns {
			gotCol := got.Columns[j]
			if gotCol.Name != wantCol.Name {
				t.Errorf("%s column %d is %q, want %q — column order is not preserved",
					want.Name, j, gotCol.Name, wantCol.Name)
			}
			// Comments are not compared: they are rendered for a person to
			// read and deliberately not parsed back. What has to survive is
			// what drives the migration.
			if gotCol.Rule.Action != wantCol.Rule.Action ||
				gotCol.Rule.Target != wantCol.Rule.Target ||
				gotCol.Rule.Transform != wantCol.Rule.Transform {
				t.Errorf("%s.%s: %s -> %q via %q, want %s -> %q via %q",
					want.Name, wantCol.Name,
					gotCol.Rule.Action, gotCol.Rule.Target, gotCol.Rule.Transform,
					wantCol.Rule.Action, wantCol.Rule.Target, wantCol.Rule.Transform)
			}
			if gotCol.Type != wantCol.Type {
				t.Errorf("%s.%s: type %q, want %q", want.Name, wantCol.Name, gotCol.Type, wantCol.Type)
			}
		}
	}
}

// A generated file that a person has since edited is the normal input, so the
// loader has to accept a reasonable edit.
func TestLoadAcceptsAHandEdit(t *testing.T) {
	const edited = `
version: 1
source:
    database: torrenttrader
    server: db:3306/torrenttrader
tables:
    users:
        action: migrate
        target: users
        columns:
            # Decided by hand: this mod column is the bonus balance.
            seedbonus:
                type: float
                action: map
                target: bonus_points
            id:
                type: int unsigned
                action: map
                target: id
`
	doc, err := Load(strings.NewReader(edited))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Tables) != 1 {
		t.Fatalf("%d tables, want 1", len(doc.Tables))
	}
	users := doc.Tables[0]
	if users.Columns[0].Name != "seedbonus" || users.Columns[0].Rule.Target != "bonus_points" {
		t.Errorf("the hand edit was not read back: %+v", users.Columns[0])
	}
}

// The version exists so a file this tool does not understand is refused rather
// than half-read.
func TestLoadRefusesWhatItDoesNotUnderstand(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"empty", "", "empty"},
		{"not YAML", "\tthis: [is: not", "not valid YAML"},
		{"no version", "tables:\n    users:\n        action: skip\n", "no version"},
		{"a future version", "version: 99\ntables:\n    users:\n        action: skip\n", "version 99"},
		{"no tables", "version: 1\ntables: {}\n", "no tables"},
		{"a table with no action", "version: 1\ntables:\n    users:\n        target: users\n", "no action"},
		{"an unknown table action", "version: 1\ntables:\n    users:\n        action: teleport\n", "unknown action"},
		{"a column with no action", "version: 1\ntables:\n    users:\n        action: migrate\n        columns:\n            id:\n                target: id\n", "no action"},
		{"an unknown column action", "version: 1\ntables:\n    users:\n        action: migrate\n        columns:\n            id:\n                action: sideways\n", "unknown action"},
		{"a scalar where a table should be", "version: 1\ntables:\n    users: nonsense\n", "must be a YAML mapping"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tc.in))
			if err == nil {
				t.Fatalf("Load(%q) returned no error", tc.in)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error is %q, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadFileReportsThePath(t *testing.T) {
	if _, err := LoadFile("testdata/no-such-mapping.yaml"); err == nil {
		t.Fatal("LoadFile on a missing file returned no error")
	} else if !strings.Contains(err.Error(), "no-such-mapping.yaml") {
		t.Errorf("the error does not name the file: %v", err)
	}
}
