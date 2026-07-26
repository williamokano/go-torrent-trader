package connector

import (
	"fmt"
	"reflect"
	"sync"
)

// TemplateField documents one thing an admin's message template may reference.
//
// It exists so the help shown next to the template box comes from the same place
// the template surface is defined. A hand-written list in the frontend would be
// a second source of truth, and the failure mode is quiet: the field list keeps
// promising something the renderer stopped providing, and the admin only finds
// out when every delivery for that instance dead-letters.
type TemplateField struct {
	// Token is what goes in the template, e.g. "{{.SizeHuman}}".
	Token string `json:"token"`
	// Description says what the field is, in one line.
	Description string `json:"description"`
	// Example is the field rendered against Sample() — literally what the token
	// produces, not a hand-written approximation of it.
	Example string `json:"example"`
}

// templateFieldDocs maps each RenderContext field to its description. Keyed by
// Go field name so the guard test below can compare it against the struct.
var templateFieldDocs = map[string]string{
	"Name":      "Torrent name, as uploaded",
	"Category":  "Category name (the torrent's own category, not its parent)",
	"Size":      "Size in bytes, unformatted — use SizeHuman unless you need the raw number",
	"SizeHuman": "Size with binary units",
	"URL":       "Link to the torrent page",
	"Link":      "The torrent name as a Markdown link to its page, shortened if very long — renders as a clickable name in the shoutbox and on Discord. Use Name and URL instead on IRC, Telegram and webhooks, which do not render Markdown",
	"Uploader":  `Uploader's name, or "` + AnonymousUploader + `" for an anonymous upload`,
	"Freeleech": "true or false — usually used as {{if .Freeleech}}…{{end}}",
	"FileCount": "Number of files in the torrent",
	// Always 0 in an instance template: when the rate limit collapses a burst,
	// every connector substitutes its own coalesced wording rather than running
	// the admin's template, so this is only reachable from those built-ins.
	"Coalesced": "Number of torrents a summary stands for — always 0 here, since coalesced summaries use their own wording",
}

// TemplateFields lists the template surface with a worked example for each
// field, in the order the fields are declared on RenderContext.
//
// Computed once: the result is deterministic (nothing on the render context is
// time-dependent) and it is read on every admin connectors page load.
var TemplateFields = sync.OnceValue(func() []TemplateField {
	sample := NewRenderContext(Sample())
	value := reflect.ValueOf(sample)
	typ := value.Type()

	fields := make([]TemplateField, 0, typ.NumField())
	for i := range typ.NumField() {
		field := typ.Field(i)
		// An unexported or embedded field is not part of the admin-facing
		// surface, and reading one through reflection would panic inside an
		// HTTP handler.
		if !field.IsExported() || field.Anonymous {
			continue
		}
		fields = append(fields, TemplateField{
			Token:       "{{." + field.Name + "}}",
			Description: templateFieldDocs[field.Name],
			Example:     fmt.Sprintf("%v", value.Field(i).Interface()),
		})
	}
	return fields
})
