package bbcode

import (
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func convert(s string) string { return Convert(s, Options{}) }

func TestInlineFormatting(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"bold", "[b]bold[/b]", "**bold**"},
		{"italic", "[i]italic[/i]", "*italic*"},
		{"underline", "[u]under[/u]", "<u>under</u>"},
		{"strikethrough", "[s]gone[/s]", "~~gone~~"},
		{"uppercase tags", "[B]bold[/B]", "**bold**"},
		{"mixed case tags", "[B]bold[/b]", "**bold**"},
		{"aliases", "[strong]a[/strong] [em]b[/em]", "**a** *b*"},
		{"text around", "before [b]bold[/b] after", "before **bold** after"},
		{"empty is dropped", "[b][/b]", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := convert(tc.in); got != tc.want {
				t.Errorf("Convert(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNesting(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"bold in italic", "[i][b]both[/b][/i]", "***both***"},
		{"italic inside bold text", "[b]bold [i]and italic[/i] again[/b]", "**bold *and italic* again**"},
		{"three deep", "[b][i][u]deep[/u][/i][/b]", "***<u>deep</u>***"},
		{"siblings", "[b]one[/b] and [b]two[/b]", "**one** and **two**"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := convert(tc.in); got != tc.want {
				t.Errorf("Convert(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The requirement the epic singles out: malformed BBCode passes through rather
// than corrupting. A stray [b] somebody can see is a smaller loss than a post
// rearranged into something nobody typed.
func TestMalformedInputPassesThrough(t *testing.T) {
	tests := []struct {
		name, in, want string
		why            string
	}{
		{
			name: "unclosed tag", in: "[b]never closed", want: `\[b\]never closed`,
			why: "emitting ** with no terminator would leave stray asterisks",
		},
		{
			name: "closing tag with nothing open", in: "orphan[/b]", want: `orphan\[/b\]`,
			why: "there is nothing for it to close",
		},
		{
			name: "crossed tags", in: "[b]a[i]b[/b]c[/i]", want: `**a\[i\]b**c\[/i\]`,
			why: "the bold is well formed and converts; the [i] it crosses was never closed, " +
				"and the [/i] left over closes nothing, so both come back as written",
		},
		{
			name: "unclosed inside closed", in: "[quote][b]broken[/quote]", want: "> \\[b\\]broken\n\n",
			why: "the quote is well-formed and converts; the bold inside it is not",
		},
		{
			name: "a bare bracket", in: "array[0] and array[1]", want: `array\[0\] and array\[1\]`,
			why: "brackets in prose were never markup",
		},
		{
			name: "an unknown tag", in: "[blink]hello[/blink]", want: `\[blink\]hello`,
			why: "an unrecognised tag was probably never markup either",
		},
		{
			name: "an empty tag", in: "[] nothing", want: `\[\] nothing`,
		},
		{
			name: "an unterminated bracket", in: "[b unterminated", want: `\[b unterminated`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convert(tc.in)
			if got != tc.want {
				t.Errorf("Convert(%q) = %q, want %q\n%s", tc.in, got, tc.want, tc.why)
			}
		})
	}
}

// A properly closed tag inside a broken one is not itself broken.
func TestWellFormedContentSurvivesABrokenParent(t *testing.T) {
	got := convert("[b]unclosed [i]closed properly[/i] more")
	want := `\[b\]unclosed *closed properly* more`
	if got != want {
		t.Errorf("Convert() = %q, want %q", got, want)
	}
}

func TestLinks(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"bare url", "[url]http://example.com[/url]", "<http://example.com>"},
		{"labelled url", "[url=http://example.com]the site[/url]", "[the site](http://example.com)"},
		{"quoted attribute", `[url="http://example.com"]x[/url]`, "[x](http://example.com)"},
		{"image", "[img]http://example.com/a.png[/img]", "![](http://example.com/a.png)"},
		{"image with attribute", "[img=http://example.com/a.png][/img]", "![](http://example.com/a.png)"},
		{"parentheses are encoded", "[url=http://example.com/a(b)]x[/url]", "[x](http://example.com/a%28b%29)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := convert(tc.in); got != tc.want {
				t.Errorf("Convert(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A URL is not prose, so the escaping applied to text must not reach it —
// underscores in a link are part of the address.
func TestURLsAreNotEscapedAsProse(t *testing.T) {
	got := convert("[url]http://example.com/a_b_c[/url]")
	if strings.Contains(got, `\_`) {
		t.Errorf("the link target was escaped as prose: %q", got)
	}
}

// Old forums collected these, and carrying one across turns a fifteen-year-old
// post into a live problem on a new site.
func TestScriptURLsAreDefused(t *testing.T) {
	for _, in := range []string{
		"[url=javascript:alert(1)]click[/url]",
		"[url=JavaScript:alert(1)]click[/url]",
		"[url=data:text/html;base64,PHNjcmlwdD4=]click[/url]",
		"[img]javascript:alert(1)[/img]",
	} {
		got := convert(in)
		lower := strings.ToLower(got)
		if strings.Contains(lower, "javascript:") || strings.Contains(lower, "data:") {
			t.Errorf("Convert(%q) = %q, which is still executable", in, got)
		}
	}
}

func TestQuotes(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		got := convert("[quote]quoted words[/quote]")
		if got != "> quoted words\n\n" {
			t.Errorf("Convert() = %q", got)
		}
	})

	t.Run("attributed", func(t *testing.T) {
		got := convert("[quote=alice]her words[/quote]")
		want := "> **alice wrote:**\n>\n> her words\n\n"
		if got != want {
			t.Errorf("Convert() = %q, want %q", got, want)
		}
	})

	t.Run("multi-line", func(t *testing.T) {
		got := convert("[quote]one\ntwo[/quote]")
		want := "> one\n> two\n\n"
		if got != want {
			t.Errorf("Convert() = %q, want %q", got, want)
		}
	})

	// A blank line inside a quote ends it unless the blank line is marked too.
	t.Run("blank line inside", func(t *testing.T) {
		got := convert("[quote]one\n\ntwo[/quote]")
		want := "> one\n>\n> two\n\n"
		if got != want {
			t.Errorf("Convert() = %q, want %q", got, want)
		}
	})

	t.Run("nested", func(t *testing.T) {
		got := convert("[quote=alice][quote=bob]deep[/quote]reply[/quote]")
		if !strings.Contains(got, "> > deep") {
			t.Errorf("the inner quote was not nested: %q", got)
		}
	})
}

func TestCode(t *testing.T) {
	t.Run("fenced", func(t *testing.T) {
		got := convert("[code]x := 1[/code]")
		want := "\n```\nx := 1\n```\n"
		if got != want {
			t.Errorf("Convert() = %q, want %q", got, want)
		}
	})

	// The whole point of [code] is that its content is not interpreted.
	t.Run("content is not parsed", func(t *testing.T) {
		got := convert("[code][b]not bold[/b][/code]")
		if !strings.Contains(got, "[b]not bold[/b]") {
			t.Errorf("the code block was interpreted: %q", got)
		}
		if strings.Contains(got, "**") {
			t.Errorf("the code block was converted: %q", got)
		}
	})

	// Escaping inside a fence would put backslashes into somebody's code.
	t.Run("content is not escaped", func(t *testing.T) {
		got := convert("[code]a_b * c[/code]")
		if strings.Contains(got, `\_`) || strings.Contains(got, `\*`) {
			t.Errorf("the code block was escaped: %q", got)
		}
	})

	// A fence that is not longer than the backticks inside it ends early and
	// swallows the rest of the post.
	t.Run("backticks inside", func(t *testing.T) {
		got := convert("[code]```\nnested\n```[/code]")
		if !strings.Contains(got, "````") {
			t.Errorf("the fence was not lengthened past the content: %q", got)
		}
	})

	t.Run("unclosed", func(t *testing.T) {
		got := convert("[code]never closed")
		if !strings.Contains(got, "never closed") {
			t.Errorf("the content was lost: %q", got)
		}
	})
}

func TestLists(t *testing.T) {
	// Each ends with a blank line, which closes the list. With one newline,
	// Markdown's lazy continuation folds whatever follows into the last bullet.
	tests := []struct{ name, in, want string }{
		{"unordered", "[list][*]one[*]two[/list]", "\n- one\n- two\n\n"},
		{"ordered", "[list=1][*]one[*]two[/list]", "\n1. one\n2. two\n\n"},
		{"ol alias", "[ol][*]one[/ol]", "\n1. one\n\n"},
		{"formatting inside", "[list][*][b]bold[/b] item[/list]", "\n- **bold** item\n\n"},
		{"empty items dropped", "[list][*][*]real[/list]", "\n- real\n\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := convert(tc.in); got != tc.want {
				t.Errorf("Convert(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// [*] has no closing form, so the last item runs to the end of the list.
	t.Run("last item ends at the list", func(t *testing.T) {
		got := convert("[list][*]one[*]two[/list]after")
		if !strings.Contains(got, "- two") || !strings.Contains(got, "after") {
			t.Errorf("Convert() = %q", got)
		}
		// This assertion used to be `Contains("- two\nafter") && !Contains("- two\n")`,
		// which can never both hold: the second substring is a prefix of the first.
		// It was dead code guarding the exact bug that shipped — a list emitting one
		// trailing newline lets Markdown's lazy continuation pull the following
		// paragraph into the last bullet. What has to be true is that a blank line
		// separates them.
		if strings.Contains(got, "- two\nafter") {
			t.Errorf("the text after the list was absorbed into the last bullet: %q", got)
		}
		if !strings.Contains(got, "\n\nafter") {
			t.Errorf("no blank line separates the list from the text after it: %q", got)
		}
	})
}

// BBCode gave no meaning to these characters, so people typed them freely.
// Markdown gives all of them meaning.
func TestPlainTextIsPreserved(t *testing.T) {
	tests := []struct {
		name, in, want string
		why            string
	}{
		{
			name: "asterisks in arithmetic", in: "2*3*4 = 24", want: `2\*3\*4 = 24`,
			why: "unescaped, the middle of the sum becomes italic",
		},
		{
			name: "underscores in a name", in: "snake_case_name", want: `snake\_case\_name`,
			why: "unescaped, part of the name becomes italic and the underscores vanish",
		},
		{
			name: "a hash at line start", in: "#1 release", want: `\#1 release`,
			why: "unescaped, the line becomes a heading",
		},
		{
			name: "a hash mid-line is left alone", in: "release #1", want: "release #1",
			why: "escaping it there would put a visible backslash into ordinary prose",
		},
		{
			name: "a dash at line start", in: "- not a bullet", want: `\- not a bullet`,
		},
		{
			name: "a number at line start", in: "1. not a list", want: `1\. not a list`,
		},
		{
			name: "a quote marker", in: "> not a quote", want: `\> not a quote`,
		},
		{
			name: "backticks", in: "use `code` here", want: "use \\`code\\` here",
		},
		{
			name: "a backslash", in: `a\b`, want: `a\\b`,
		},
		{
			name: "an angle bracket", in: "5 < 6", want: `5 \< 6`,
			why: "unescaped it can open an HTML tag",
		},
		{
			name: "a pipe", in: "a | b", want: `a \| b`,
			why: "unescaped it can start a table cell",
		},
		{
			name: "ordinary prose is untouched", in: "Just a normal sentence.", want: "Just a normal sentence.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convert(tc.in)
			if got != tc.want {
				t.Errorf("Convert(%q) = %q, want %q\n%s", tc.in, got, tc.want, tc.why)
			}
		})
	}
}

// Latin1 text is what the corpus is full of, and it must come through untouched.
func TestNonASCIISurvives(t *testing.T) {
	for _, in := range []string{
		"Café, naïve, Größe, façade",
		"Le Fabuleux Destin d'Amélie",
		"日本語のテキスト",
		"Ελληνικά",
	} {
		if got := convert(in); got != in {
			t.Errorf("Convert(%q) = %q, want it unchanged", in, got)
		}
	}
}

func TestPreserveReturnsTheInputUnchanged(t *testing.T) {
	in := "[b]bold[/b] and 2*3"
	if got := Convert(in, Options{Preserve: true}); got != in {
		t.Errorf("Convert(preserve) = %q, want %q", got, in)
	}
}

func TestEmptyInput(t *testing.T) {
	if got := convert(""); got != "" {
		t.Errorf("Convert(\"\") = %q", got)
	}
}

// Presentation the target cannot express is dropped, but never the words.
func TestPresentationTagsKeepTheirText(t *testing.T) {
	for _, in := range []string{
		"[color=red]important[/color]",
		"[size=24]big[/size]",
		"[center]middle[/center]",
		"[font=Arial]typed[/font]",
	} {
		got := convert(in)
		if !strings.Contains(got, "important") && !strings.Contains(got, "big") &&
			!strings.Contains(got, "middle") && !strings.Contains(got, "typed") {
			t.Errorf("Convert(%q) = %q, which lost the words", in, got)
		}
		if strings.Contains(got, "[") {
			t.Errorf("Convert(%q) = %q, which kept the tag", in, got)
		}
	}
}

// Whatever the input, the conversion has to terminate and produce something.
// These are the shapes that make a hand-rolled parser loop or panic.
func TestPathologicalInputIsSurvivable(t *testing.T) {
	inputs := []string{
		strings.Repeat("[b]", 500),
		strings.Repeat("[/b]", 500),
		strings.Repeat("[b]x[/b]", 500),
		strings.Repeat("[quote]", 200) + "deep" + strings.Repeat("[/quote]", 200),
		"[",
		"]",
		"[]",
		"[/]",
		"[=]",
		"[b=]x[/b]",
		"[url=]x[/url]",
		"[list]" + strings.Repeat("[*]", 200) + "[/list]",
		strings.Repeat("[", 1000),
		"\x00\x01\x02",
	}
	for i, in := range inputs {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			// A panic or a hang here is the failure; the output only has to
			// exist.
			_ = convert(in)
		})
	}
}

// The exact strings the test corpus carries, so the converter is checked
// against the content the migration will actually meet.
func TestCorpusContent(t *testing.T) {
	tests := []struct{ name, in string }{
		{"simple markup", "[b]Bold[/b] and [i]italic[/i]."},
		{"unclosed and nested", "[quote][b]unclosed bold [i]and italic[/b] still going[/quote] [url=http://example.com]link"},
		{"a quoted reply", "[quote=alice]Please read the rules.[/quote] Done."},
		{"latin1 with markup", "Je regarde [i]Amélie[/i] — très bien !"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convert(tc.in)
			if got == "" {
				t.Fatal("the content was lost entirely")
			}
			// Nothing may silently vanish: every run of letters in the input
			// has to appear in the output.
			for _, word := range []string{"Bold", "italic", "unclosed", "rules", "Amélie", "link", "Done"} {
				if strings.Contains(tc.in, word) && !strings.Contains(got, word) {
					t.Errorf("%q lost the word %q: %q", tc.in, word, got)
				}
			}
		})
	}
}

// The conversion runs once over text nobody will proofread, so a case that comes
// out *plausible but wrong* is worse than one that fails loudly. Each of these
// produced readable output that said something the member did not write.
func TestSilentCorruptionsFoundInReview(t *testing.T) {
	tests := []struct {
		name, in, want, why string
	}{{
		name: "a code body whose rune case-folds to a different byte length",
		in:   "[code]K[/code]TAIL",
		want: "\n```\nK\n```\nTAIL",
		why: "indexFold searched a ToLower'd copy and sliced the original with the " +
			"offset, so the body was cut mid-rune and the output was invalid UTF-8 — " +
			"which PostgreSQL rejects outright, failing the migration on one post",
	}, {
		name: "a bare url containing markup characters",
		in:   "[url]http://a.com/><b>x</b>[/url]",
		want: "<http://a.com/%3E%3Cb%3Ex%3C/b%3E>",
		why: "the autolink form is <target>, so an unencoded > closed it early and " +
			"everything after became raw HTML — a live element out of a member's post",
	}, {
		name: "text after a quote",
		in:   "[quote]a[/quote]after",
		want: "> a\n\nafter",
		why: "one trailing newline let lazy continuation pull the next paragraph " +
			"into the quote, attributing the member's own words to whoever they quoted",
	}, {
		name: "a space inside an emphasis tag",
		in:   "[b] hello [/b]",
		want: " **hello** ",
		why: "Markdown will not open emphasis on a marker followed by whitespace, so " +
			"this rendered as the literal ** hello ** — and it is very common input",
	}, {
		name: "the same emphasis nested",
		in:   "[i][i]x[/i][/i]",
		want: "*x*",
		why:  "the doubled marker read as bold, which is not what was written",
	}, {
		name: "an image with dimensions in the attribute",
		in:   "[img=100,80]http://a.com/pic.jpg[/img]",
		want: "![](http://a.com/pic.jpg)",
		why:  "the attribute won unconditionally, so the URL vanished from the post",
	}, {
		name: "a tilde in prose",
		in:   "~~~",
		want: `\~\~\~`,
		why: "GFM reads ~~~ as a code fence, swallowing the rest of the post — the " +
			"same failure the backtick fence rule prevents, reached through prose",
	}, {
		name: "an indented line",
		in:   "    four space indent",
		want: `    \four space indent`,
		why:  "four spaces makes an indented code block; pasted logs and ASCII art are everywhere",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := convert(tc.in); got != tc.want {
				t.Errorf("Convert(%q) = %q, want %q\n%s", tc.in, got, tc.want, tc.why)
			}
		})
	}
}

// Nesting *different* emphasis must keep both, or the fix for the nested-identical
// case above would be silently dropping formatting instead.
func TestNestingDifferentEmphasisKeepsBoth(t *testing.T) {
	for _, in := range []string{"[i][b]both[/b][/i]", "[b][i]both[/i][/b]"} {
		if got := convert(in); got != "***both***" {
			t.Errorf("Convert(%q) = %q, want %q", in, got, "***both***")
		}
	}
}

// Every scheme the old deny-list missed, and every legitimate target it allowed.
// Browsers strip control characters from URLs and CommonMark decodes entity
// references in a link destination, so the three prefix tests it did were
// bypassable five ways.
func TestOnlyLinkSchemesSurvive(t *testing.T) {
	defused := []string{
		"java\tscript:alert(1)", "java\vscript:alert(1)", "java\x00script:alert(1)",
		"\x01javascript:alert(1)", "&#106;avascript:alert(1)", "JaVaScRiPt:alert(1)",
		"vbscript:msgbox", "data:text/html,<script>", "file:///etc/passwd",
	}
	for _, target := range defused {
		t.Run("defused "+target, func(t *testing.T) {
			got := convert("[url=" + target + "]x[/url]")
			if got != "[x](#)" {
				t.Errorf("Convert() = %q, want the target defused to #", got)
			}
		})
		t.Run("defused img "+target, func(t *testing.T) {
			if got := convert("[img]" + target + "[/img]"); got != "![](#)" {
				t.Errorf("Convert() = %q, want the target defused to #", got)
			}
		})
	}

	kept := map[string]string{
		"https://ok.example/a?b=1&c=2": "[x](https://ok.example/a?b=1&c=2)",
		"http://ok.example":            "[x](http://ok.example)",
		"mailto:a@b.com":               "[x](mailto:a@b.com)",
		"ftp://files.example/pub":      "[x](ftp://files.example/pub)",
		"/relative/path":               "[x](/relative/path)",
		"page.php?id=1":                "[x](page.php?id=1)",
	}
	for target, want := range kept {
		t.Run("kept "+target, func(t *testing.T) {
			if got := convert("[url=" + target + "]x[/url]"); got != want {
				t.Errorf("Convert() = %q, want %q — a legitimate target was broken", got, want)
			}
		})
	}
}

// A real property, unlike the hardcoded word list above: every run of letters and
// digits in the input must survive into the output, whatever the markup around it.
//
// The list version was satisfiable by construction — seven words checked against
// four inputs, each gated on `strings.Contains(tc.in, word)`, so an input that
// never contained them asserted nothing. That is why it stayed green while
// `[code]K[/code]TAIL` mangled TAIL into "e]TAIL" and `[spoiler]x[/spoiler]` ate
// its closing tag. Deriving the expectation from the input catches the class
// rather than the instances.
func TestNoWordEverGoesMissing(t *testing.T) {
	inputs := []string{
		"[b]Bold[/b] and [i]italic[/i].",
		"[quote][b]unclosed bold [i]and italic[/b] still going[/quote] tail",
		"[quote=alice]Please read the rules.[/quote] Done.",
		"Je regarde [i]Amélie[/i] — très bien !",
		"[code]KELVIN[/code]TAIL",
		"[code]dotted[/code]AFTER",
		"[url=http://example.com]anchor text[/url] afterwards",
		"[img=100,80]http://a.com/pic.jpg[/img]",
		"[list][*]alpha[*]beta[/list]gamma",
		"[b] spaced [/b] words",
		"2*3*4 and snake_case_name and #1 release",
		"[unknowntag]inner words[/unknowntag] outer",
		"~~struck~~ and ~tilde~",
		"    indented line",
		"[quote]quoted[/quote]immediately",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			got := convert(in)
			for _, word := range wordsIn(in) {
				if !strings.Contains(got, word) {
					t.Errorf("the word %q is missing from the output\n  in:  %q\n  out: %q",
						word, in, got)
				}
			}
		})
	}
}

// wordsIn returns the runs of letters and digits in s, skipping BBCode tag names
// so that a converted [b] is not reported as a lost word. Runs shorter than three
// characters are skipped too: single letters collide with markup by chance.
func wordsIn(s string) []string {
	// Blank out anything between brackets — the markup, not the prose.
	stripped := []rune(s)
	depth := 0
	for i, r := range stripped {
		switch r {
		case '[':
			depth++
			stripped[i] = ' '
		case ']':
			if depth > 0 {
				depth--
			}
			stripped[i] = ' '
		default:
			if depth > 0 {
				stripped[i] = ' '
			}
		}
	}

	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() >= 3 {
			words = append(words, cur.String())
		}
		cur.Reset()
	}
	for _, r := range stripped {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return words
}
