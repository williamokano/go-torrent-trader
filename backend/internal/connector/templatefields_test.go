package connector

import (
	"reflect"
	"strings"
	"testing"
)

// This is the test the whole TemplateField type exists for. The tokens are
// derived from RenderContext by reflection, so listing them proves nothing —
// what has to hold is that the hand-written descriptions cover exactly the
// fields this build renders. Add a field without describing it and the help
// silently stops being the truth, so it fails here instead.
func TestEveryRenderContextFieldIsDocumented(t *testing.T) {
	described := make(map[string]string)
	for _, field := range TemplateFields() {
		described[field.Token] = field.Description
	}

	typ := reflect.TypeOf(RenderContext{})
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() || f.Anonymous {
			continue
		}
		token := "{{." + f.Name + "}}"
		if strings.TrimSpace(described[token]) == "" {
			t.Errorf("%s is renderable but has no description", token)
		}
	}
}

// The other direction: a description for a field that no longer exists would
// promise the admin a token that fails to render.
func TestNoDescriptionOutlivesItsField(t *testing.T) {
	typ := reflect.TypeOf(RenderContext{})
	for name := range templateFieldDocs {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("templateFieldDocs describes %q, which is not a render-context field", name)
		}
	}
}

// The examples are only worth showing if they are what the token really
// produces, so they are rendered rather than written down.
func TestTemplateFieldExamplesComeFromTheRenderer(t *testing.T) {
	for _, field := range TemplateFields() {
		rendered, err := RenderTemplate(field.Token, Sample())
		if err != nil {
			t.Fatalf("rendering %s: %v", field.Token, err)
		}
		if rendered != field.Example {
			t.Errorf("%s documents example %q but renders %q",
				field.Token, field.Example, rendered)
		}
	}
}
