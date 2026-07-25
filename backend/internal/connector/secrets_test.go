package connector

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

var secretFields = []string{"hmac_secret", "bot_token"}

func TestRedactConfigStripsSecretsAndReportsWhichAreSet(t *testing.T) {
	cfg := json.RawMessage(`{"url":"https://example.test/hook","hmac_secret":"s3cr3t","bot_token":""}`)

	redacted, set := RedactConfig(cfg, secretFields)

	if strings.Contains(string(redacted), "s3cr3t") {
		t.Fatalf("redacted config still contains the secret: %s", redacted)
	}
	if strings.Contains(string(redacted), "hmac_secret") {
		t.Fatalf("redacted config still contains the secret key: %s", redacted)
	}
	if !strings.Contains(string(redacted), "example.test") {
		t.Fatalf("redaction dropped a non-secret key: %s", redacted)
	}
	// bot_token is present but empty, so it is not "set".
	if len(set) != 1 || set[0] != "hmac_secret" {
		t.Fatalf("secrets_set = %v, want [hmac_secret]", set)
	}
}

// An unparseable config could hide a secret anywhere, so nothing at all is
// returned rather than a partially-redacted object.
func TestRedactConfigRefusesToPartiallyDiscloseUnparseableConfig(t *testing.T) {
	redacted, set := RedactConfig(json.RawMessage(`not json`), secretFields)

	if string(redacted) != "{}" {
		t.Fatalf("redacted = %s, want {}", redacted)
	}
	if set != nil {
		t.Fatalf("secrets_set = %v, want nil", set)
	}
}

func TestMergeSecretsOmittedKeyKeepsStoredValue(t *testing.T) {
	existing := json.RawMessage(`{"url":"https://old.test","hmac_secret":"stored"}`)
	incoming := json.RawMessage(`{"url":"https://new.test"}`)

	merged, err := MergeSecrets(existing, incoming, secretFields)
	if err != nil {
		t.Fatalf("MergeSecrets: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	if got["hmac_secret"] != "stored" {
		t.Fatalf("hmac_secret = %v, want the stored value", got["hmac_secret"])
	}
	if got["url"] != "https://new.test" {
		t.Fatalf("url = %v, want the incoming value", got["url"])
	}
}

// The write-only form always submits a blank secret field, so blank has to mean
// "leave it alone" — otherwise every edit would wipe the credential.
func TestMergeSecretsEmptyStringKeepsStoredValue(t *testing.T) {
	existing := json.RawMessage(`{"hmac_secret":"stored"}`)
	incoming := json.RawMessage(`{"hmac_secret":""}`)

	merged, err := MergeSecrets(existing, incoming, secretFields)
	if err != nil {
		t.Fatalf("MergeSecrets: %v", err)
	}
	if !strings.Contains(string(merged), `"hmac_secret":"stored"`) {
		t.Fatalf("merged = %s, want the stored secret kept", merged)
	}
}

func TestMergeSecretsNonEmptyReplaces(t *testing.T) {
	existing := json.RawMessage(`{"hmac_secret":"stored"}`)
	incoming := json.RawMessage(`{"hmac_secret":"rotated"}`)

	merged, err := MergeSecrets(existing, incoming, secretFields)
	if err != nil {
		t.Fatalf("MergeSecrets: %v", err)
	}
	if !strings.Contains(string(merged), `"hmac_secret":"rotated"`) {
		t.Fatalf("merged = %s, want the new secret", merged)
	}
}

// Explicit null is the only way to clear a credential, since blank means keep.
func TestMergeSecretsExplicitNullClears(t *testing.T) {
	existing := json.RawMessage(`{"hmac_secret":"stored"}`)
	incoming := json.RawMessage(`{"hmac_secret":null}`)

	merged, err := MergeSecrets(existing, incoming, secretFields)
	if err != nil {
		t.Fatalf("MergeSecrets: %v", err)
	}
	if strings.Contains(string(merged), "hmac_secret") {
		t.Fatalf("merged = %s, want hmac_secret removed", merged)
	}
	if strings.Contains(string(merged), "stored") {
		t.Fatalf("merged = %s, still contains the cleared secret", merged)
	}
}

func TestMergeSecretsBlankWithNoStoredValueDropsTheKey(t *testing.T) {
	merged, err := MergeSecrets(json.RawMessage(`{}`), json.RawMessage(`{"hmac_secret":""}`), secretFields)
	if err != nil {
		t.Fatalf("MergeSecrets: %v", err)
	}
	if strings.Contains(string(merged), "hmac_secret") {
		t.Fatalf("merged = %s, want the empty secret dropped rather than stored", merged)
	}
}

func TestMergeSecretsRejectsMalformedInput(t *testing.T) {
	if _, err := MergeSecrets(json.RawMessage(`{}`), json.RawMessage(`nope`), secretFields); err == nil {
		t.Fatal("expected malformed incoming config to be rejected")
	}
}

// The whole point of the redaction pass: a secret that leaked into an error
// string must not reach a log line or the delivery log.
func TestRedactErrorScrubsSecretValues(t *testing.T) {
	cfg := json.RawMessage(`{"bot_token":"123:AAHsuperSecret"}`)
	err := fmt.Errorf("Post \"https://api.telegram.org/bot123:AAHsuperSecret/sendMessage\": connection refused")

	got := RedactError(err, cfg, secretFields)

	if strings.Contains(got, "AAHsuperSecret") {
		t.Fatalf("redacted error still contains the token: %s", got)
	}
	if !strings.Contains(got, Redacted) {
		t.Fatalf("redacted error = %q, want a %s marker", got, Redacted)
	}
	if !strings.Contains(got, "connection refused") {
		t.Fatalf("redacted error lost the useful part: %q", got)
	}
}

func TestRedactErrorNilReturnsEmpty(t *testing.T) {
	if got := RedactError(nil, nil, secretFields); got != "" {
		t.Fatalf("RedactError(nil) = %q, want empty", got)
	}
}

// If the config cannot be parsed we cannot know which substrings are secret, so
// the message is dropped rather than published.
func TestRedactErrorDropsMessageWhenConfigIsUnreadable(t *testing.T) {
	got := RedactError(errors.New("token=AAHsuperSecret rejected"), json.RawMessage(`not json`), secretFields)

	if strings.Contains(got, "AAHsuperSecret") {
		t.Fatalf("unredactable error leaked: %s", got)
	}
}

// A connector with no secrets keeps its error text verbatim.
func TestRedactErrorWithoutSecretFieldsPassesThrough(t *testing.T) {
	got := RedactError(errors.New("boom"), json.RawMessage(`bad`), nil)
	if got != "boom" {
		t.Fatalf("RedactError = %q, want %q", got, "boom")
	}
}

func TestRedactStringTruncatesVeryLongErrors(t *testing.T) {
	long := strings.Repeat("x", maxErrorLen*2)

	got := RedactString(long, json.RawMessage(`{}`), nil)

	if len(got) > maxErrorLen+len("…") {
		t.Fatalf("len(redacted) = %d, want it capped near %d", len(got), maxErrorLen)
	}
}
