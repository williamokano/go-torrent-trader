// Package compare diffs a discovered legacy schema against the TorrentTrader
// 3.0 baseline and says what an operator needs to look at before a migration.
//
// The premise, from the epic: no two TorrentTrader installs are identical after
// years of mods, so the tool reports what it found rather than assuming. A
// difference is therefore not automatically a failure. Only two things stop a
// run — a required table that is absent, and an absent column the migration
// actually reads — because those are the cases where it has nothing to read.
//
// The second half of that matters. The mapping skips 36 baseline columns
// outright: age, gender, signature, the timezone offset, the who's-online
// scratch field. An install that dropped one years ago migrates perfectly, and
// an earlier version of this package refused to run over it.
package compare

import (
	"sort"
	"strings"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/baseline"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
)

// TypeMismatch is a column whose type differs from the baseline.
type TypeMismatch struct {
	Column   string
	Baseline string
	Found    string
}

// TableReport is the diff for one table present in both the baseline and the
// source database.
type TableReport struct {
	Name string
	// ColumnsChecked is false when the baseline knows the table but not its
	// columns, in which case the column fields are empty and mean nothing.
	ColumnsChecked bool
	// MissingColumns are absent columns the migration reads. These block.
	MissingColumns []string
	// DroppedColumns are absent baseline columns the migration does not read.
	// Worth reporting, since they say something about the install, but
	// harmless.
	DroppedColumns []string
	// AddedColumns are source columns the baseline does not have — almost
	// always a mod.
	AddedColumns []string
	// TypeMismatches are columns present in both with different types.
	TypeMismatches []TypeMismatch
}

// Differs reports whether anything about the table is worth printing.
func (t TableReport) Differs() bool {
	return len(t.MissingColumns) > 0 || len(t.DroppedColumns) > 0 ||
		len(t.AddedColumns) > 0 || len(t.TypeMismatches) > 0
}

// Report is the whole comparison.
type Report struct {
	// MissingTables are baseline tables absent from the source.
	MissingTables []string
	// MissingRequiredTables is the subset of MissingTables the migration
	// cannot run without.
	MissingRequiredTables []string
	// AddedTables are source tables the baseline does not know — mods, or
	// another application sharing the schema.
	AddedTables []string
	// Tables holds a report per table present in both, in sorted order.
	Tables []TableReport
}

// Blocking reports whether the differences prevent a migration from running: a
// required table is gone, or a column the transformers read is gone. Added
// columns, type mismatches and dropped columns nobody reads are not blocking —
// they are what the mapping file exists to resolve.
func (r Report) Blocking() bool {
	if len(r.MissingRequiredTables) > 0 {
		return true
	}
	for _, t := range r.Tables {
		if len(t.MissingColumns) > 0 {
			return true
		}
	}
	return false
}

// Clean reports whether the source is a stock TorrentTrader 3.0 schema.
func (r Report) Clean() bool {
	if len(r.MissingTables) > 0 || len(r.AddedTables) > 0 {
		return false
	}
	for _, t := range r.Tables {
		if t.Differs() {
			return false
		}
	}
	return true
}

// Compare diffs found against base.
//
// reads maps a legacy table name to the columns the migration reads from it —
// mapping.ReadColumns() supplies it. A nil map means "assume every baseline
// column is read", which is the safe reading for a caller that has no plan to
// hand, not the useful one.
func Compare(base baseline.Schema, found schema.Schema, reads map[string][]string) Report {
	var r Report

	for _, bt := range base.Tables {
		ft, ok := found.Table(bt.Name)
		if !ok {
			r.MissingTables = append(r.MissingTables, bt.Name)
			if bt.Required {
				r.MissingRequiredTables = append(r.MissingRequiredTables, bt.Name)
			}
			continue
		}
		r.Tables = append(r.Tables, compareTable(bt, ft, reads))
	}

	for _, ft := range found.Tables {
		if _, ok := base.Table(ft.Name); !ok {
			r.AddedTables = append(r.AddedTables, ft.Name)
		}
	}

	sort.Strings(r.MissingTables)
	sort.Strings(r.MissingRequiredTables)
	sort.Strings(r.AddedTables)
	sort.Slice(r.Tables, func(i, j int) bool { return r.Tables[i].Name < r.Tables[j].Name })
	return r
}

func compareTable(bt baseline.Table, ft schema.Table, reads map[string][]string) TableReport {
	report := TableReport{Name: bt.Name, ColumnsChecked: bt.ColumnsDocumented}
	if !bt.ColumnsDocumented {
		return report
	}
	read := readSet(reads, bt.Name)

	for _, bc := range bt.Columns {
		fc, ok := ft.Column(bc.Name)
		if !ok {
			if read == nil || read[strings.ToLower(bc.Name)] {
				report.MissingColumns = append(report.MissingColumns, bc.Name)
			} else {
				report.DroppedColumns = append(report.DroppedColumns, bc.Name)
			}
			continue
		}
		if !schema.TypesEqual(bc.Type, fc.Type) {
			report.TypeMismatches = append(report.TypeMismatches, TypeMismatch{
				Column:   bc.Name,
				Baseline: bc.Type,
				Found:    fc.Type,
			})
		}
	}

	for _, fc := range ft.Columns {
		if _, ok := bt.Column(fc.Name); !ok {
			report.AddedColumns = append(report.AddedColumns, fc.Name)
		}
	}
	return report
}

// readSet returns the columns the migration reads from a table, as a set keyed
// by lower-cased name. A nil result means "no information", which callers treat
// as "every column is read".
func readSet(reads map[string][]string, table string) map[string]bool {
	if reads == nil {
		return nil
	}
	for name, columns := range reads {
		if !strings.EqualFold(name, table) {
			continue
		}
		set := make(map[string]bool, len(columns))
		for _, c := range columns {
			set[strings.ToLower(c)] = true
		}
		return set
	}
	// The table has a baseline entry but the plan reads nothing from it.
	return map[string]bool{}
}
