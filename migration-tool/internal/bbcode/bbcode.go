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
	for _, n := range nodes {
		renderNode(b, n)
	}
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
		// An unknown tag was probably never markup. Put it back as written.
		b.WriteString(escapeMarkdown(n.raw))
		renderNodes(b, n.children)
	}
}

func wrap(b *strings.Builder, n *node, marker string) {
	var inner strings.Builder
	renderNodes(&inner, n.children)

	// Emphasis with nothing in it renders as literal asterisks, so an empty
	// [b][/b] is dropped instead.
	if strings.TrimSpace(inner.String()) == "" {
		b.WriteString(inner.String())
		return
	}
	b.WriteString(marker)
	b.WriteString(inner.String())
	b.WriteString(marker)
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
	src := n.attr
	if src == "" {
		src = rawText(n)
	}
	if src == "" {
		return
	}
	b.WriteString("![](")
	b.WriteString(sanitizeURL(src))
	b.WriteString(")")
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
	b.WriteString("\n")
}

func renderCode(b *strings.Builder, n *node) {
	body := rawText(n)
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
			b.WriteString(itoa(i + 1))
			b.WriteString(". ")
		} else {
			b.WriteString("- ")
		}
		// A multi-line item has to stay indented under its bullet.
		b.WriteString(strings.ReplaceAll(item, "\n", "\n  "))
		b.WriteString("\n")
	}
}

// rawText returns a node's text with no escaping, for the places where the
// content is a URL or code rather than prose.
func rawText(n *node) string {
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
	return strings.TrimSpace(b.String())
}

// sanitizeURL keeps a link target from breaking out of the Markdown syntax
// holding it, and refuses the schemes that make a link a script.
func sanitizeURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.NewReplacer("\n", "", "\r", "", " ", "%20", "(", "%28", ")", "%29").Replace(u)

	switch {
	case strings.HasPrefix(strings.ToLower(u), "javascript:"),
		strings.HasPrefix(strings.ToLower(u), "data:"),
		strings.HasPrefix(strings.ToLower(u), "vbscript:"):
		// Legacy forums collected these. Carrying one across turns an old
		// post into a live problem on a new site.
		return "#"
	}
	return u
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
