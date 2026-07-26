package mapping

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads a mapping file.
//
// The file is hand-edited between being generated and being used, which is the
// whole point of it — so this has to cope with what a person leaves behind, and
// say clearly what is wrong rather than half-reading a broken file and starting
// a migration on it.
//
// Column order is preserved. It is not needed for correctness, but a file that
// comes back in a different order than it went out is one nobody can diff.
func Load(r io.Reader) (Document, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return Document{}, fmt.Errorf("reading the mapping: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return Document{}, fmt.Errorf("the mapping is not valid YAML: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return Document{}, fmt.Errorf("the mapping is empty")
	}
	return parseDocument(root.Content[0])
}

// LoadFile reads a mapping from a path.
func LoadFile(path string) (Document, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the operator's own argument
	if err != nil {
		return Document{}, fmt.Errorf("opening the mapping: %w", err)
	}
	defer func() { _ = f.Close() }()

	doc, err := Load(f)
	if err != nil {
		return Document{}, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

func parseDocument(node *yaml.Node) (Document, error) {
	var doc Document
	if node.Kind != yaml.MappingNode {
		return doc, fmt.Errorf("the mapping must be a YAML mapping at the top level")
	}

	for key, value := range pairs(node) {
		switch strings.ToLower(key) {
		case "version":
			if err := value.Decode(&doc.Version); err != nil {
				return doc, fmt.Errorf("version is not a number: %w", err)
			}
		case "source":
			for k, v := range pairs(value) {
				switch strings.ToLower(k) {
				case "database":
					doc.Database = v.Value
				case "server":
					doc.Server = v.Value
				}
			}
		case "tables":
			tables, err := parseTables(value)
			if err != nil {
				return doc, err
			}
			doc.Tables = tables
		}
	}

	// A reader that does not recognise the version must refuse the file
	// rather than guess at what an unknown field means.
	if doc.Version == 0 {
		return doc, fmt.Errorf("the mapping has no version; it was not written by this tool")
	}
	if doc.Version != Version {
		return doc, fmt.Errorf("the mapping is version %d and this tool understands version %d — regenerate it with `migrate mapping`", doc.Version, Version)
	}
	if len(doc.Tables) == 0 {
		return doc, fmt.Errorf("the mapping has no tables")
	}
	return doc, nil
}

func parseTables(node *yaml.Node) ([]Table, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("tables must be a YAML mapping of table name to definition")
	}

	var tables []Table
	for name, body := range pairs(node) {
		t, err := parseTable(name, body)
		if err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, nil
}

func parseTable(name string, node *yaml.Node) (Table, error) {
	t := Table{Name: name}
	if node.Kind != yaml.MappingNode {
		return t, fmt.Errorf("table %s: definition must be a YAML mapping", name)
	}

	for key, value := range pairs(node) {
		switch strings.ToLower(key) {
		case "action":
			t.Action = TableAction(value.Value)
		case "target":
			t.Target = value.Value
		case "columns":
			columns, err := parseColumns(name, value)
			if err != nil {
				return t, err
			}
			t.Columns = columns
		case "derived":
			t.Derived = map[string]string{}
			for k, v := range pairs(value) {
				t.Derived[k] = v.Value
			}
		case "absent_columns":
			for _, item := range value.Content {
				t.Absent = append(t.Absent, item.Value)
			}
		}
	}

	switch t.Action {
	case TableMigrate, TableSkip, TableReview:
	case "":
		return t, fmt.Errorf("table %s: no action", name)
	default:
		return t, fmt.Errorf("table %s: unknown action %q", name, t.Action)
	}
	return t, nil
}

func parseColumns(table string, node *yaml.Node) ([]Column, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("table %s: columns must be a YAML mapping", table)
	}

	var columns []Column
	for name, body := range pairs(node) {
		c := Column{Name: name}
		if body.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s.%s: definition must be a YAML mapping", table, name)
		}
		for key, value := range pairs(body) {
			switch strings.ToLower(key) {
			case "type":
				c.Type = value.Value
			case "charset":
				c.Charset = value.Value
			case "action":
				c.Rule.Action = Action(value.Value)
			case "target":
				c.Rule.Target = value.Value
			case "transform":
				c.Rule.Transform = value.Value
			}
		}

		switch c.Rule.Action {
		case ActionMap, ActionDerive, ActionSkip, ActionCustom, ActionReview:
		case "":
			return nil, fmt.Errorf("%s.%s: no action", table, name)
		default:
			return nil, fmt.Errorf("%s.%s: unknown action %q", table, name, c.Rule.Action)
		}
		columns = append(columns, c)
	}
	return columns, nil
}

// pairs iterates a YAML mapping node in document order, yielding keys exactly
// as written — table and column names keep their case, and only field names are
// compared case-insensitively, by the callers that do so.
//
// yaml.v3 keeps a mapping's Content as alternating key and value nodes, which
// is what makes order recoverable at all: decoding into a Go map throws it
// away, and a file that comes back reordered is one nobody can diff.
func pairs(node *yaml.Node) func(func(string, *yaml.Node) bool) {
	return func(yield func(string, *yaml.Node) bool) {
		if node == nil || node.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if !yield(node.Content[i].Value, node.Content[i+1]) {
				return
			}
		}
	}
}
