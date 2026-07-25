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
