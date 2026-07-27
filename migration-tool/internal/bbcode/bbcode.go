// Package bbcode converts TorrentTrader's BBCode to Markdown.
//
// Every piece of member-written content in the legacy site is BBCode —
// descriptions, forum posts, private messages, comments — and this project is
// Markdown-only. So this runs over text people wrote, once, with no chance to
// check the result by hand.
//
// That shapes every decision here. Malformed input passes through as written
// rather than being guessed at, because a stray [b] in a fifteen-year-old post
// is a smaller loss than a post rearranged into something nobody typed. Text
// that was never markup is escaped, because Markdown gives meaning to
// characters BBCode did not: a post reading 2*3*4 must not silently become
// italic on arrival.
package bbcode

import (
	"html"
	"strconv"
	"strings"
)

// Options configures a conversion.
type Options struct {
	// Preserve returns the text unchanged. It exists for an operator who
	// intends to render BBCode in the new site rather than convert it —
	// the conversion is lossy in one direction only, and nothing can undo it
	// afterwards.
	Preserve bool
}

// Convert turns BBCode into Markdown.
func Convert(input string, opts Options) string {
	if opts.Preserve || input == "" {
		return input
	}

	var b strings.Builder
	renderNodes(&b, parse(tokenize(input)))
	return b.String()
}

func renderNodes(b *strings.Builder, nodes []*node) {
	for _, n := range mergeAdjacentEmphasis(nodes) {
		renderNode(b, n)
	}
}

// emphasisTags are the tags rendered by wrapping their content in a marker. Two of
// them side by side produce two markers side by side, which Markdown reads as one
// longer marker rather than as a close followed by an open.
var emphasisTags = map[string]bool{
	"b": true, "strong": true, "i": true, "em": true,
	"s": true, "strike": true, "del": true, "spoiler": true, "u": true,
}

// mergeAdjacentEmphasis folds neighbouring runs of the same emphasis into one.
//
// `[i]a[/i][i]b[/i]` rendered as `*a**b*`, and the `**` in the middle is read as a
// bold delimiter, so the output was `<em>a**b</em>` — the second run lost its italics
// and gained two literal asterisks. Merging is not an approximation: two adjacent
// italic runs with nothing between them *are* one italic run, so `*ab*` says exactly
// what the member wrote.
//
// Only direct neighbours merge. `[i]a[/i] [i]b[/i]` has a text node between them and
// is left alone, which is right — the space is content, and the two markers are no
// longer adjacent.
func mergeAdjacentEmphasis(nodes []*node) []*node {
	if len(nodes) < 2 {
		return nodes
	}
	out := make([]*node, 0, len(nodes))
	for _, n := range nodes {
		if len(out) > 0 {
			prev := out[len(out)-1]
			if n.tag != "" && prev.tag == n.tag && prev.attr == n.attr && emphasisTags[n.tag] {
				// Copy rather than mutate: the same node may be reachable from
				// elsewhere in the tree, and a render must not rewrite the parse.
				merged := *prev
				merged.children = append(append([]*node{}, prev.children...), n.children...)
				out[len(out)-1] = &merged
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

func renderNode(b *strings.Builder, n *node) {
	if n.tag == "" {
		b.WriteString(escapeMarkdown(n.text))
		return
	}

	switch n.tag {
	case "b", "strong":
		wrap(b, n, "**")
	case "i", "em":
		wrap(b, n, "*")
	case "s", "strike", "del":
		wrap(b, n, "~~")
	case "spoiler":
		// The target implements !!…!! (frontend/src/components/remarkSpoiler.ts), and
		// [spoiler] is in the legacy supported-tag list, so this is a convertible tag
		// rather than an unknown one. It used to fall through to the default branch,
		// which emitted the opener literally and silently swallowed the closer.
		wrap(b, n, "!!")
	case "u":
		// Markdown has no underline. HTML is the only way to keep the
		// intent, and if the renderer strips it the words still survive —
		// which dropping the tag outright would also do, but without the
		// chance of it working.
		b.WriteString("<u>")
		renderNodes(b, n.children)
		b.WriteString("</u>")
	case "url":
		renderURL(b, n)
	case "img":
		renderImage(b, n)
	case "quote":
		renderQuote(b, n)
	case "code":
		renderCode(b, n)
	case "list", "ul", "ol":
		renderList(b, n)
	case "*":
		renderNodes(b, n.children)
	case "color", "size", "font", "center", "left", "right", "align":
		// Presentation the target has no way to express. The words are what
		// matter; the formatting is dropped rather than approximated.
		renderNodes(b, n.children)
	default:
		// An unknown tag was probably never markup. Put it back as written — all of
		// it. Only the opener was emitted before, so `[spoiler]hidden[/spoiler]`
		// became `\[spoiler\]hidden`: neither converted nor passed through, which
		// are the only two defensible outcomes for something this does not
		// understand. A node still carrying a tag here was definitely closed, since
		// the parser literalizes the unclosed ones, so emitting the closer cannot
		// invent one that was not typed.
		b.WriteString(escapeMarkdown(n.raw))
		renderNodes(b, n.children)
		b.WriteString(escapeMarkdown("[/" + n.tag + "]"))
	}
}

func wrap(b *strings.Builder, n *node, marker string) {
	var inner strings.Builder
	renderNodes(&inner, n.children)

	// Emphasis with nothing in it renders as literal asterisks, so an empty
	// [b][/b] is dropped instead.
	body := inner.String()
	if strings.TrimSpace(body) == "" {
		b.WriteString(body)
		return
	}

	// Markdown will not open emphasis on a marker followed by whitespace, nor
	// close one on a marker preceded by it — so "[b] hello [/b]" rendered as the
	// literal "** hello **". A space inside the tag is extremely common in
	// hand-typed posts. Hoisting the whitespace outside the markers keeps both the
	// spacing and the formatting.
	lead := body[:len(body)-len(strings.TrimLeft(body, " \t\n"))]
	tail := body[len(strings.TrimRight(body, " \t\n")):]
	body = body[len(lead) : len(body)-len(tail)]

	// [i][i]x[/i][/i] doubles the marker into "**x**", which is *bold* — the one
	// thing the member did not write. A repeat of the same emphasis adds nothing,
	// so the inner one is dropped rather than concatenated.
	// An *exact* run, not a prefix: for [i] wrapping [b], the body starts with "**"
	// where the marker is "*", and that is a different emphasis rather than a
	// repeat of this one — dropping it would turn "***both***" into "**both**".
	if markerRun(body, marker[0]) == len(marker) &&
		markerRunEnd(body, marker[0]) == len(marker) &&
		len(body) > 2*len(marker) {
		b.WriteString(lead)
		b.WriteString(body)
		b.WriteString(tail)
		return
	}

	b.WriteString(lead)
	b.WriteString(marker)
	b.WriteString(body)
	b.WriteString(marker)
	b.WriteString(tail)
}

func renderURL(b *strings.Builder, n *node) {
	var inner strings.Builder
	renderNodes(&inner, n.children)
	text := inner.String()

	// [url=target]text[/url]
	if n.attr != "" {
		b.WriteString("[")
		b.WriteString(text)
		b.WriteString("](")
		b.WriteString(sanitizeURL(n.attr))
		b.WriteString(")")
		return
	}

	// [url]target[/url] — the text is the target, so it must not carry the
	// escaping applied to prose.
	raw := rawText(n)
	if raw == "" {
		return
	}
	b.WriteString("<")
	b.WriteString(sanitizeURL(raw))
	b.WriteString(">")
}

func renderImage(b *strings.Builder, n *node) {
	// [img=url] carries the target in the attribute, but [img=width,height] carries
	// *dimensions* there and the target in the body. Taking the attribute
	// unconditionally turned "[img=100,80]http://a.com/pic.jpg[/img]" into
	// "![](100,80)" — the image lost, the URL gone from the post entirely.
	src := n.attr
	if src == "" || isImageDimensions(src) {
		if body := rawText(n); body != "" {
			src = body
		}
	}
	if src == "" {
		return
	}
	b.WriteString("![](")
	b.WriteString(sanitizeURL(src))
	b.WriteString(")")
}

// markerRun counts the leading run of c, and markerRunEnd the trailing one.
func markerRun(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

func markerRunEnd(s string, c byte) int {
	n := 0
	for n < len(s) && s[len(s)-1-n] == c {
		n++
	}
	return n
}

// isImageDimensions reports whether an [img=...] attribute is a size rather than a
// URL: digits separated by a comma or an x, as legacy forums wrote it.
func isImageDimensions(attr string) bool {
	seenDigit, seenSep := false, false
	for i := 0; i < len(attr); i++ {
		switch c := attr[i]; {
		case c >= '0' && c <= '9':
			seenDigit = true
		case c == ',' || c == 'x' || c == 'X' || c == ' ':
			seenSep = true
		default:
			return false
		}
	}
	return seenDigit && seenSep
}

func renderQuote(b *strings.Builder, n *node) {
	var inner strings.Builder
	if n.attr != "" {
		inner.WriteString("**")
		inner.WriteString(escapeMarkdown(n.attr))
		inner.WriteString(" wrote:**\n\n")
	}
	renderNodes(&inner, n.children)

	body := strings.TrimSpace(inner.String())
	if body == "" {
		return
	}
	// A block quote needs every line marked, including the blank ones, or the
	// quote ends at the first empty line.
	for i, line := range strings.Split(body, "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		if line == "" {
			b.WriteString(">")
			continue
		}
		b.WriteString("> ")
		b.WriteString(line)
	}
	// A blank line, not one newline. With a single "\n", Markdown's lazy
	// continuation pulls the following paragraph *into* the quote:
	// "[quote]a[/quote]after" rendered as one blockquote reading "a after",
	// silently attributing the member's own words to whoever they were quoting.
	b.WriteString("\n\n")
}

func renderCode(b *strings.Builder, n *node) {
	body := rawTextPreservingIndent(n)
	if strings.TrimSpace(body) == "" {
		return
	}
	body = strings.Trim(body, "\n")

	// A fence has to be longer than the longest run of backticks inside it,
	// or the block ends early and the rest of the post is swallowed.
	fence := strings.Repeat("`", max(3, longestBacktickRun(body)+1))

	b.WriteString("\n")
	b.WriteString(fence)
	b.WriteString("\n")
	b.WriteString(body)
	b.WriteString("\n")
	b.WriteString(fence)
	b.WriteString("\n")
}

func renderList(b *strings.Builder, n *node) {
	ordered := n.tag == "ol" || n.attr == "1"

	var items []string
	var loose []*node
	for _, child := range n.children {
		if child.tag == "*" {
			var item strings.Builder
			renderNodes(&item, child.children)
			items = append(items, strings.TrimSpace(item.String()))
			continue
		}
		loose = append(loose, child)
	}

	// Text between [list] and the first [*] is not part of any item.
	var before strings.Builder
	renderNodes(&before, loose)
	if text := strings.TrimSpace(before.String()); text != "" {
		b.WriteString(text)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	for i, item := range items {
		if item == "" {
			continue
		}
		if ordered {
			b.WriteString(strconv.Itoa(i + 1))
			b.WriteString(". ")
		} else {
			b.WriteString("- ")
		}
		// A multi-line item has to stay indented under its bullet.
		b.WriteString(strings.ReplaceAll(item, "\n", "\n  "))
		b.WriteString("\n")
	}
	// Closes the list, for the same reason the quote above needs it: otherwise
	// "[list][*]a[/list]after" renders "after" inside the last bullet.
	b.WriteString("\n")
}

// rawText returns a node's text with no escaping, for the places where the
// content is a URL or code rather than prose.
// rawText returns a node's text with no escaping.
//
// trimTrailing controls whether surrounding whitespace goes. A URL wants it gone;
// a [code] block does not — TrimSpace stripped the *first* line's indentation while
// leaving every later line intact, so pasted code arrived with its alignment broken,
// which is most of the reason someone reached for [code] at all.
func rawTextPreservingIndent(n *node) string {
	return collectRawText(n)
}

func rawText(n *node) string {
	return strings.TrimSpace(collectRawText(n))
}

func collectRawText(n *node) string {
	var b strings.Builder
	var walk func(*node)
	walk = func(x *node) {
		if x.tag == "" {
			b.WriteString(x.text)
			return
		}
		if x.raw != "" && x.tag != "code" {
			b.WriteString(x.raw)
		}
		for _, c := range x.children {
			walk(c)
		}
	}
	for _, c := range n.children {
		walk(c)
	}
	return b.String()
}

// allowedURLSchemes are the schemes a migrated link may carry.
//
// An allow-list, not a deny-list. The deny-list this replaces tested three
// prefixes and was bypassable several ways at once — `java\tscript:`,
// `java\vscript:`, `java\x00script:`, `\x01javascript:` and `&#106;avascript:` all
// sailed through, because browsers strip tabs, newlines and leading control
// characters from URLs and CommonMark decodes entity references inside a link
// destination. Enumerating what is dangerous cannot work when the attacker picks
// the spelling; enumerating what is useful can.
var allowedURLSchemes = map[string]bool{
	"http": true, "https": true, "mailto": true, "ftp": true, "ftps": true,
}

// sanitizeURL keeps a link target from breaking out of the Markdown syntax holding
// it, and refuses any scheme that is not plainly a link.
func sanitizeURL(u string) string {
	// Control characters first, and dropped rather than encoded: a browser
	// ignores them inside a URL, so leaving them in is what let a scheme be
	// spelled around the check. Covers NUL, tab, vertical tab, form feed, CR, LF
	// and DEL in one pass.
	u = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, u)
	u = strings.TrimSpace(u)

	if !schemeIsAllowed(u) {
		return "#"
	}

	// `<` and `>` are the pair that mattered and were missing. The bare
	// [url]target[/url] form renders as an autolink, `<target>`, so a `>` in the
	// target closed it early and everything after it became raw HTML in the
	// output — `[url]http://a/><b>bold</b>[/url]` produced a live <b> element out
	// of a member's post, and a <script> when the renderer had no sanitizer. That
	// contradicts the claim that <u> is the only HTML this converter emits, and the
	// conversion is one-way, so it would have been stored that way for good.
	return strings.NewReplacer(
		" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E",
	).Replace(u)
}

// schemeIsAllowed reports whether a target's scheme is one we carry across.
//
// A target with no scheme is relative and fine. The comparison is made against an
// entity-decoded copy, because CommonMark decodes entity references in a link
// destination — so `&#106;avascript:` reaches the browser as `javascript:` — but
// only the copy is decoded, since decoding the target itself would corrupt a
// legitimate `?a=1&amp;b=2` query string.
func schemeIsAllowed(u string) bool {
	probe := strings.ToLower(html.UnescapeString(u))

	colon := strings.IndexByte(probe, ':')
	if colon < 0 {
		return true
	}
	// A colon inside a path, query or fragment is not a scheme separator:
	// "/a:b" and "?x=a:b" are ordinary relative targets.
	for _, sep := range []byte{'/', '?', '#'} {
		if i := strings.IndexByte(probe, sep); i >= 0 && i < colon {
			return true
		}
	}
	return allowedURLSchemes[probe[:colon]]
}

func longestBacktickRun(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
			continue
		}
		run = 0
	}
	return longest
}
