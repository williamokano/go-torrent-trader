// Package mention extracts @username mentions from user-authored text. It is the
// single source of truth for what counts as a mention on the backend; the
// frontend MarkdownEditor typeahead mirrors the same rule.
package mention

import "regexp"

// mentionRegex matches an @username preceded by the start of the string,
// whitespace, or an opening parenthesis. Usernames are word characters
// ([A-Za-z0-9_]), matching the registration rule.
var mentionRegex = regexp.MustCompile(`(?:^|[\s(])@(\w+)`)

// Extract returns the usernames mentioned in body, in order of appearance
// (duplicates preserved; callers dedupe by resolved user). Returns an empty
// slice when there are no mentions.
func Extract(body string) []string {
	matches := mentionRegex.FindAllStringSubmatch(body, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}
