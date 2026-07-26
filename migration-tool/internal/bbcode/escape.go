package bbcode

import "strings"

// Escaping is the half of this conversion that is easy to forget and changes
// what a member's words mean.
//
// BBCode gives no meaning to * or _ or #, so people typed them freely: "2*3*4",
// "snake_case_name", "#1 release". Markdown gives all of them meaning. Copying
// the text across unescaped silently italicises part of a sentence, turns a
// line into a heading, or eats a character entirely — and nobody is going to
// read a million migrated posts to find out.
//
// So text that was never markup is escaped so it renders as it was written.
// Content that is markup — a URL, the inside of a code block — is not, because
// escaping it there would corrupt it instead.

// inlineSpecials are the characters Markdown interprets anywhere in a line.
var inlineSpecials = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	`*`, `\*`,
	`_`, `\_`,
	`[`, `\[`,
	`]`, `\]`,
	`<`, `\<`,
	`|`, `\|`,
	// GFM reads ~~x~~ as strikethrough and ~~~ as a code fence — the same
	// swallow-the-rest-of-the-post failure the backtick fence rule was written to
	// prevent, reachable through prose instead. BBCode gave ~ no meaning, so a
	// 2009 post using it for emphasis or ASCII art hits both.
	`~`, `\~`,
)

// escapeMarkdown makes a run of legacy text render as written.
func escapeMarkdown(s string) string {
	if s == "" {
		return s
	}

	escaped := inlineSpecials.Replace(s)

	// The rest only mean something at the start of a line, and escaping them
	// mid-sentence would put visible backslashes into ordinary prose.
	var b strings.Builder
	b.Grow(len(escaped) + 8)
	for i, line := range strings.Split(escaped, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(escapeLineStart(line))
	}
	return b.String()
}

// escapeLineStart guards the constructs Markdown only recognises at the
// beginning of a line: headings, quotes, list bullets and numbered items.
func escapeLineStart(line string) string {
	indent := 0
	for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
		indent++
	}
	prefix, rest := line[:indent], line[indent:]
	if rest == "" {
		return line
	}

	// Four spaces, or one tab, makes an indented code block. Indented ASCII art,
	// pasted logs and NFO fragments are everywhere in fifteen years of posts, and
	// they arrive as a grey monospace box with the indentation eaten. The
	// indentation is what the member typed, so it is kept and made inert by
	// escaping the first visible character rather than by trimming.
	if isIndentedCodeBlock(prefix) {
		return prefix + `\` + rest
	}

	switch rest[0] {
	case '#', '>', '+':
		return prefix + `\` + rest
	case '-':
		// A run of dashes is a horizontal rule or a setext underline; a
		// single one followed by a space is a bullet. Either way it has to
		// stop being either.
		return prefix + `\` + rest
	case '=':
		// Underlines the line above into a heading.
		if allSame(rest, '=') {
			return prefix + `\` + rest
		}
	}

	// "1." and "1)" start an ordered list.
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits < len(rest) && (rest[digits] == '.' || rest[digits] == ')') {
		return prefix + rest[:digits] + `\` + rest[digits:]
	}
	return line
}

// isIndentedCodeBlock reports whether leading whitespace is enough to open one.
// A tab counts as four columns, which is what CommonMark does.
func isIndentedCodeBlock(prefix string) bool {
	columns := 0
	for i := 0; i < len(prefix); i++ {
		if prefix[i] == '\t' {
			columns += 4
			continue
		}
		columns++
	}
	return columns >= 4
}

func allSame(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return len(s) > 0
}
