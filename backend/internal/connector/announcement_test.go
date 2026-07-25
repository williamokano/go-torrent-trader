package connector

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestEventKeyIsPerTorrent(t *testing.T) {
	a := publishedAnnouncement()
	if got, want := EventKey(a), "torrent.published:42"; got != want {
		t.Fatalf("EventKey = %q, want %q", got, want)
	}
}

func TestRenderTemplateDefaultsWhenBlank(t *testing.T) {
	a := publishedAnnouncement()
	a.URL = "https://tracker.test/torrent/42"

	got, err := RenderTemplate("", a)
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}
	for _, want := range []string{a.Name, a.Category, "2.00 GiB", a.URL} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered %q, missing %q", got, want)
		}
	}
}

func TestRenderTemplateCustom(t *testing.T) {
	a := publishedAnnouncement()
	a.Freeleech = true

	got, err := RenderTemplate("{{.Name}}{{if .Freeleech}} [FL]{{end}} by {{.Uploader}}", a)
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}
	if want := "Some.Release-GROUP [FL] by alice"; got != want {
		t.Fatalf("rendered %q, want %q", got, want)
	}
}

// {{.Link}} exists so an admin does not have to write "[{{.Name}}]({{.URL}})"
// by hand, which breaks the moment a name contains brackets — and scene names
// contain brackets constantly.
func TestMarkdownLinkEscapesBothSides(t *testing.T) {
	tests := []struct {
		name string
		text string
		url  string
		want string
	}{
		{
			name: "plain",
			text: "Some.Release-GROUP",
			url:  "https://tracker.test/torrent/42",
			want: "[Some.Release-GROUP](https://tracker.test/torrent/42)",
		},
		{
			name: "brackets in the label would close it early",
			text: "[SubsPlease] Show - 01",
			url:  "https://tracker.test/t/1",
			want: `[\[SubsPlease\] Show - 01](https://tracker.test/t/1)`,
		},
		{
			name: "parens and emphasis markers in the label",
			text: "Show (2024) *REPACK* _v2_ ~old~ `raw`",
			url:  "https://tracker.test/t/2",
			want: "[Show \\(2024\\) \\*REPACK\\* \\_v2\\_ \\~old\\~ \\`raw\\`](https://tracker.test/t/2)",
		},
		{
			name: "a backslash is escaped once, not twice over",
			text: `back\slash`,
			url:  "https://tracker.test/t/3",
			want: `[back\\slash](https://tracker.test/t/3)`,
		},
		{
			name: "parens in the destination would end it early",
			text: "Name",
			url:  "https://tracker.test/t/(4)",
			want: "[Name](https://tracker.test/t/%284%29)",
		},
		{
			// A label pointing nowhere is worse than no link: it renders as
			// literal brackets around the name.
			name: "no url yields the escaped text alone",
			text: "[Name]",
			url:  "  ",
			want: `\[Name\]`,
		},
		{
			// Torrent names are uploader-controlled. Left raw, this renders as a
			// second link to an attacker-chosen host inside a row that reads as
			// an official site announcement.
			name: "an autolink in the name cannot open a second link",
			text: "Ping <https://evil.example.com> now",
			url:  "https://tracker.test/t/5",
			want: `[Ping \<https://evil.example.com\> now](https://tracker.test/t/5)`,
		},
		{
			name: "raw HTML in the name is inert",
			text: `Auto <a href='https://evil.example.com'>x</a>`,
			url:  "https://tracker.test/t/6",
			want: `[Auto \<a href='https://evil.example.com'\>x\</a\>](https://tracker.test/t/6)`,
		},
		{
			name: "an entity in the name stays literal",
			text: "Tom &amp; Jerry",
			url:  "https://tracker.test/t/7",
			want: `[Tom \&amp; Jerry](https://tracker.test/t/7)`,
		},
		{
			// ![alt](url) inside the label would otherwise become an image.
			name: "image syntax in the name is inert",
			text: "Show ![x](https://evil.example.com/p.png)",
			url:  "https://tracker.test/t/8",
			want: `[Show \!\[x\]\(https://evil.example.com/p.png\)](https://tracker.test/t/8)`,
		},
		{
			// A newline reads as a paragraph break, which splits the link in
			// half and leaves the URL bare.
			name: "whitespace is collapsed",
			text: "Para1\n\nPara2\tend",
			url:  "https://tracker.test/t/9",
			want: "[Para1 Para2 end](https://tracker.test/t/9)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MarkdownLink(tc.text, tc.url); got != tc.want {
				t.Fatalf("MarkdownLink(%q, %q) = %q, want %q", tc.text, tc.url, got, tc.want)
			}
		})
	}
}

func TestRenderTemplateLinkField(t *testing.T) {
	a := publishedAnnouncement()
	a.Name = "[Group] Thing"
	a.URL = "https://tracker.test/torrent/42"

	got, err := RenderTemplate("{{.Link}}", a)
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}
	if want := `[\[Group\] Thing](https://tracker.test/torrent/42)`; got != want {
		t.Fatalf("rendered %q, want %q", got, want)
	}
}

// The shared default feeds IRC and Announcement.Body, where Markdown is noise.
// Only the shoutbox connector opts into the linked form.
func TestSharedDefaultTemplateStaysPlainText(t *testing.T) {
	if strings.Contains(DefaultTemplate, "{{.Link}}") {
		t.Fatalf("shared DefaultTemplate = %q, must stay plain text", DefaultTemplate)
	}
}

// A template referencing a field the render context does not expose must fail,
// not quietly render "<no value>" into every announcement.
func TestRenderTemplateRejectsUnknownField(t *testing.T) {
	if _, err := RenderTemplate("{{.Titel}}", publishedAnnouncement()); err == nil {
		t.Fatal("expected a template with an unknown field to fail")
	}
}

func TestValidateTemplate(t *testing.T) {
	if err := ValidateTemplate(""); err != nil {
		t.Fatalf("blank template must be allowed: %v", err)
	}
	if err := ValidateTemplate("{{.Name}}"); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}
	if err := ValidateTemplate("{{.Name"); err == nil {
		t.Fatal("expected an unterminated action to be rejected")
	}
}

// A field typo parses fine and only fails when rendered, so validation has to
// render too — otherwise the template saves cleanly and then dead-letters every
// announcement for that instance until someone reads the log.
func TestValidateTemplateRejectsFieldTypo(t *testing.T) {
	err := ValidateTemplate("New: {{.Titel}}")
	if err == nil {
		t.Fatal("expected a template referencing an unknown field to be rejected at save time")
	}
	if !strings.Contains(err.Error(), "Titel") {
		t.Fatalf("error = %v, want it to name the offending field", err)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KiB"},
		{1536, "1.50 KiB"},
		{1024 * 1024, "1.00 MiB"},
		{3 * 1024 * 1024 * 1024, "3.00 GiB"},
	}
	for _, c := range cases {
		if got := HumanSize(c.in); got != c.want {
			t.Errorf("HumanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSampleIsSelfDescribing(t *testing.T) {
	a := Sample()
	if a.Event != EventTest {
		t.Fatalf("Sample().Event = %q, want %q", a.Event, EventTest)
	}
	if a.Body == "" || a.Title == "" {
		t.Fatal("Sample() must come pre-rendered so a connector with no template still says something")
	}
}

// DeliveryKey is a transport detail, not part of the event, so it must not end
// up in the stored payload or the webhook body.
func TestDeliveryKeyIsNotSerialized(t *testing.T) {
	a := publishedAnnouncement()
	a.DeliveryKey = "torrent.published:42"

	body, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "delivery_key") {
		t.Fatalf("announcement JSON leaked the delivery key: %s", body)
	}
}

type panickingConnector struct{}

func (panickingConnector) Kind() string                         { return "boom" }
func (panickingConnector) Singleton() bool                      { return false }
func (panickingConnector) SecretFields() []string               { return nil }
func (panickingConnector) Coalescable() bool                    { return true }
func (panickingConnector) ValidateConfig(json.RawMessage) error { return nil }
func (panickingConnector) Deliver(context.Context, Instance, Announcement) error {
	panic("third-party client blew up")
}

// A panic inside a connector must become a failed delivery, not take the worker
// process (and every other queued task) down with it.
func TestSafeDeliverConvertsPanicToError(t *testing.T) {
	err := SafeDeliver(context.Background(), panickingConnector{}, Instance{}, publishedAnnouncement())
	if err == nil {
		t.Fatal("expected a panicking connector to yield an error")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("error = %q, want it to name the panic", err)
	}
}

func TestRatePerMin(t *testing.T) {
	cases := []struct {
		name string
		cfg  string
		want int
	}{
		{"absent", `{}`, DefaultRatePerMin},
		{"blank config", ``, DefaultRatePerMin},
		{"explicit", `{"rate_per_min":5}`, 5},
		// A misconfigured 0 must not mean "never announce anything".
		{"zero falls back", `{"rate_per_min":0}`, DefaultRatePerMin},
		{"negative falls back", `{"rate_per_min":-3}`, DefaultRatePerMin},
		{"unparseable falls back", `not json`, DefaultRatePerMin},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RatePerMin(json.RawMessage(c.cfg)); got != c.want {
				t.Fatalf("RatePerMin(%s) = %d, want %d", c.cfg, got, c.want)
			}
		})
	}
}

func TestValidateRatePerMin(t *testing.T) {
	if err := ValidateRatePerMin(json.RawMessage(`{"rate_per_min":10}`)); err != nil {
		t.Fatalf("valid rate rejected: %v", err)
	}
	if err := ValidateRatePerMin(json.RawMessage(`{"rate_per_min":0}`)); err == nil {
		t.Fatal("expected rate_per_min 0 to be rejected at save time")
	}
}

func TestDecodeConfigToleratesUnknownKeys(t *testing.T) {
	// rate_per_min is parsed by the pipeline, not by any connector's own config
	// struct, so tolerance here is a requirement rather than laxity.
	var cfg struct {
		Template string `json:"template"`
	}
	if err := DecodeConfig(json.RawMessage(`{"template":"x","rate_per_min":5}`), &cfg); err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if cfg.Template != "x" {
		t.Fatalf("template = %q, want %q", cfg.Template, "x")
	}
}

// The shoutbox refuses a message over 500 bytes, and the label is a torrent
// name: uploader-controlled, up to 255 characters, and doubled in length by
// escaping. Without a bound, an adversarially punctuated name would render an
// announcement its own destination rejects — the delivery burns its retries and
// dead-letters, and only that torrent is affected, so it would look like a
// one-off.
func TestMarkdownLinkBoundsAnAdversarialLabel(t *testing.T) {
	// The worst case for escaping: every character costs two bytes.
	worst := strings.Repeat("_", 255)

	got := MarkdownLink(worst, "https://tracker.test/torrent/424242")
	if len(got) > maxLinkLabelBytes+len("https://tracker.test/torrent/424242")+len("[]()") {
		t.Fatalf("link is %d bytes, longer than the label budget allows", len(got))
	}
	if !strings.Contains(got, linkLabelEllipsis) {
		t.Fatalf("a truncated label must say so: %q", got)
	}

	// Multi-byte runes are not escapable, so a rune count would have bounded
	// this at three times the intended size.
	cjk := MarkdownLink(strings.Repeat("運", 255), "https://tracker.test/t/1")
	if len(cjk) > maxLinkLabelBytes+len("https://tracker.test/t/1")+len("[]()") {
		t.Fatalf("CJK label is %d bytes — the budget is being counted in runes", len(cjk))
	}

	// A name that comfortably fits is passed through whole, ellipsis-free.
	short := "Some.Release.2024.1080p.BluRay-GROUP"
	if got := MarkdownLink(short, "https://tracker.test/t/2"); got != "["+short+"](https://tracker.test/t/2)" {
		t.Fatalf("a short name must not be touched: %q", got)
	}
}

// The end-to-end version of the bound: the line the shoutbox actually receives
// has to survive its own length check.
func TestDefaultChatLineFitsTheShoutboxLimit(t *testing.T) {
	a := publishedAnnouncement()
	a.Name = strings.Repeat("_", 255)
	a.Category = strings.Repeat("c", 64)
	a.URL = "https://a-rather-long-tracker-hostname.example.com/torrent/4294967296"

	line, err := RenderTemplate("New torrent: {{.Link}} — {{.Category}}, {{.SizeHuman}}", a)
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}
	// maxChatMessageLength in internal/service; duplicated rather than imported
	// because connector must not depend on service.
	if len(line) > 500 {
		t.Fatalf("rendered line is %d bytes, over the 500-byte shoutbox limit: %q", len(line), line)
	}
}
