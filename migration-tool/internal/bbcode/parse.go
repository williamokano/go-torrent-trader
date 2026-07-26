package bbcode

import "strings"

// The parser is a tokenizer and a stack, not a set of regular expressions.
//
// Regular expressions cannot express "this closing tag matches that opening
// one", which is the only question that matters here: fifteen years of
// hand-typed posts contain crossed tags, tags nobody closed, and stray
// brackets that were never markup at all. A regexp pass over that produces
// text nobody typed, and a mangled post is worse than a visible [b].

type kind int

const (
	kindText kind = iota
	kindOpen
	kindClose
)

// token is one lexical unit: a run of text, an opening tag, or a closing tag.
type token struct {
	kind kind
	// tag is lower-cased, so [B] and [/b] match.
	tag string
	// attr is the part after "=" in [url=http://example.com].
	attr string
	// raw is exactly what was written, so a tag that turns out to be
	// malformed can be put back untouched.
	raw string
	// text is the content of a text token.
	text string
}

// node is a parsed element. A node with an empty tag is text.
type node struct {
	tag      string
	attr     string
	text     string
	children []*node
	// raw is the opening tag as written, kept so an unclosed tag can be
	// emitted literally rather than guessed at.
	raw string
}

// tokenize splits input into tags and text.
func tokenize(input string) []token {
	var tokens []token
	var text strings.Builder

	flush := func() {
		if text.Len() > 0 {
			tokens = append(tokens, token{kind: kindText, text: text.String()})
			text.Reset()
		}
	}

	for i := 0; i < len(input); {
		if input[i] != '[' {
			text.WriteByte(input[i])
			i++
			continue
		}

		tok, width, ok := parseTag(input[i:])
		if !ok {
			// Not a tag. A bare bracket is ordinary text, and plenty of
			// posts contain one.
			text.WriteByte(input[i])
			i++
			continue
		}

		flush()
		tokens = append(tokens, tok)
		i += width

		// [code] takes its content verbatim: a post explaining BBCode by
		// showing it must not have the example interpreted.
		if tok.kind == kindOpen && tok.tag == "code" {
			body, consumed, closed := scanVerbatim(input[i:], "code")
			tokens = append(tokens, token{kind: kindText, text: body})
			i += consumed
			if closed {
				tokens = append(tokens, token{kind: kindClose, tag: "code", raw: "[/code]"})
			}
		}
	}
	flush()
	return tokens
}

// parseTag reads a tag at the start of s, returning it and how much it consumed.
func parseTag(s string) (token, int, bool) {
	end := strings.IndexByte(s, ']')
	if end < 0 {
		return token{}, 0, false
	}
	inner := s[1:end]
	if inner == "" {
		return token{}, 0, false
	}

	closing := false
	if inner[0] == '/' {
		closing = true
		inner = inner[1:]
	}

	name, attr, hasAttr := strings.Cut(inner, "=")
	if closing && hasAttr {
		return token{}, 0, false
	}
	if !isTagName(name) {
		return token{}, 0, false
	}

	t := token{
		tag:  strings.ToLower(name),
		attr: strings.Trim(attr, `"'`),
		raw:  s[:end+1],
	}
	if closing {
		t.kind = kindClose
	} else {
		t.kind = kindOpen
	}
	return t, end + 1, true
}

// isTagName reports whether s could be a BBCode tag name. Anything else inside
// brackets is text somebody typed, not markup.
func isTagName(s string) bool {
	if s == "" {
		return false
	}
	if s == "*" {
		return true
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

// scanVerbatim reads up to the closing tag without interpreting anything.
func scanVerbatim(s, tag string) (body string, consumed int, closed bool) {
	closer := "[/" + tag + "]"
	if idx := indexFold(s, closer); idx >= 0 {
		return s[:idx], idx + len(closer), true
	}
	// Never closed: the rest of the text is the body, and the caller emits
	// no closing token so the opener is treated as malformed.
	return s, len(s), false
}

func indexFold(s, substr string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(substr))
}

// selfClosing are tags with no closing form. [*] ends at the next [*] or at the
// end of the list holding it.
var selfClosing = map[string]bool{"*": true}

// parse builds a tree from tokens.
//
// Two rules give the "pass through rather than corrupt" behaviour the epic
// asks for, and everything else follows from them:
//
//   - a closing tag closes the nearest matching open tag, and anything left
//     open in between becomes text. [quote][b]x[/quote] is extremely common,
//     and the quote is perfectly good — only the [b] is broken, so only the
//     [b] is given back as written
//   - a closing tag matching nothing at all is not markup, so it becomes text
//   - an opening tag still unclosed at the end is not markup either — but its
//     children are still parsed, since a properly closed tag inside a broken
//     one is not itself broken
func parse(tokens []token) []*node {
	root := &node{}
	stack := []*node{root}

	top := func() *node { return stack[len(stack)-1] }
	push := func(n *node) {
		top().children = append(top().children, n)
		stack = append(stack, n)
	}
	// popTo unwinds to a tag, turning everything it passes into literal text
	// because those tags were never closed.
	pop := func() *node {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return n
	}

	for _, tok := range tokens {
		switch tok.kind {
		case kindText:
			top().children = append(top().children, &node{text: tok.text})

		case kindOpen:
			if selfClosing[tok.tag] {
				// [*] closes any list item already open.
				if top().tag == "*" {
					pop()
				}
				push(&node{tag: tok.tag, attr: tok.attr, raw: tok.raw})
				continue
			}
			push(&node{tag: tok.tag, attr: tok.attr, raw: tok.raw})

		case kindClose:
			// A list may close with an item still open. That is how [*]
			// works, not a mistake, so it closes properly.
			if top().tag == "*" && tok.tag != "*" {
				pop()
			}

			match := -1
			for i := len(stack) - 1; i > 0; i-- {
				if stack[i].tag == tok.tag {
					match = i
					break
				}
			}
			if match < 0 {
				// Nothing this closes. It is text.
				top().children = append(top().children, &node{text: tok.raw})
				continue
			}
			// Anything still open above the match was never closed.
			for len(stack)-1 > match {
				n := pop()
				literalize(top(), n)
			}
			pop()
		}
	}

	// Whatever is still open was never closed.
	for len(stack) > 1 {
		n := pop()
		literalize(top(), n)
	}
	return root.children
}

// literalize replaces a node in its parent with the tag as it was written,
// followed by whatever it contained. The children keep their own structure: a
// tag that was closed properly inside a broken one is not itself broken.
func literalize(parent, n *node) {
	if len(parent.children) > 0 && parent.children[len(parent.children)-1] == n {
		parent.children = parent.children[:len(parent.children)-1]
	}
	parent.children = append(parent.children, &node{text: n.raw})
	parent.children = append(parent.children, n.children...)
}
