package postgres

import (
	"strings"
	"unicode"
)

// BuildPrefixQuery converts user input into a tsquery with prefix matching.
// "forum rules" → "forum:* & rules:*"
// "日本語" → "日本語:*"
// Special characters are stripped to prevent tsquery syntax errors.
// Unicode letters and digits are preserved.
func BuildPrefixQuery(search string) string {
	words := strings.Fields(search)
	var parts []string
	for _, w := range words {
		cleaned := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return -1
		}, w)
		if cleaned != "" {
			parts = append(parts, cleaned+":*")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " & ")
}
