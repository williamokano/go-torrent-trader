package connector

import (
	"encoding/json"
	"strings"
)

// Redacted is what replaces a secret value wherever one might otherwise surface.
const Redacted = "[redacted]"

// maxErrorLen caps a stored/logged delivery error. A remote endpoint that
// answers with a megabyte of HTML should not become a megabyte of delivery-log
// row. Truncation happens after redaction, so it can never expose a secret that
// redaction would have removed.
const maxErrorLen = 1000

// minRedactableSecretLen is the shortest value worth substring-matching. Below
// it the match is more likely to be coincidence than credential.
const minRedactableSecretLen = 6

// RedactConfig strips every secret key out of a config object and reports which
// of them actually held a value. The API returns the redacted form plus that
// list, which is what lets the admin UI show "•••• set — leave blank to keep"
// without the browser ever receiving the credential.
//
// A config that cannot be parsed is returned as an empty object: a secret could
// be anywhere inside it, so partial disclosure is not an option.
func RedactConfig(cfg json.RawMessage, secretFields []string) (json.RawMessage, []string) {
	obj, ok := decodeObject(cfg)
	if !ok {
		return json.RawMessage("{}"), nil
	}
	var set []string
	for _, field := range secretFields {
		value, present := obj[field]
		if !present {
			continue
		}
		delete(obj, field)
		if nonEmptyString(value) {
			set = append(set, field)
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return json.RawMessage("{}"), nil
	}
	return out, set
}

// MergeSecrets folds an incoming config update onto the stored one, resolving
// each secret field by what the admin submitted:
//
//	absent or ""  → keep the stored value (the normal case: the form never had it)
//	non-empty     → replace
//	explicit null → clear
//
// Non-secret keys always come from the incoming config, so removing an optional
// key still works. Validation runs on the merged result, never on the raw input.
func MergeSecrets(existing, incoming json.RawMessage, secretFields []string) (json.RawMessage, error) {
	merged, err := decodeObjectErr(incoming, "config")
	if err != nil {
		return nil, err
	}
	stored, err := decodeObjectErr(existing, "stored config")
	if err != nil {
		return nil, err
	}

	for _, field := range secretFields {
		value, present := merged[field]
		switch {
		// isNull must be tested first: unmarshaling JSON null into a string
		// succeeds and leaves it empty, so "clear" would otherwise be
		// indistinguishable from "keep".
		case present && isNull(value):
			delete(merged, field)
		case !present, isEmptyString(value):
			if prev, ok := stored[field]; ok {
				merged[field] = prev
			} else {
				delete(merged, field)
			}
		}
	}

	out, err := json.Marshal(merged)
	if err != nil {
		return nil, wrapErr("marshal merged config", err)
	}
	return out, nil
}

// RedactError renders an error for storage in connector_deliveries.last_error or
// for a log line, with any configured secret value scrubbed out of it. Callers
// should still avoid building secrets into errors in the first place — this is
// the backstop for the cases that are hard to see, like a Telegram bot token
// living inside the request URL that net/http echoes back in *url.Error.
func RedactError(err error, cfg json.RawMessage, secretFields []string) string {
	if err == nil {
		return ""
	}
	return RedactString(err.Error(), cfg, secretFields)
}

// RedactString scrubs every configured secret value out of s.
func RedactString(s string, cfg json.RawMessage, secretFields []string) string {
	if len(secretFields) > 0 {
		obj, ok := decodeObject(cfg)
		if !ok {
			// The config is unreadable, so we cannot know which substrings are
			// secret. Dropping the message loses debugging detail; keeping it
			// risks publishing a credential. Drop it.
			return "[error redacted: connector config unreadable]"
		}
		for _, field := range secretFields {
			var value string
			raw, present := obj[field]
			if !present || json.Unmarshal(raw, &value) != nil {
				continue
			}
			// A very short secret would match all over an unrelated message and
			// shred it. Anything that short is not a credential worth protecting
			// at the cost of an unreadable delivery log; the save-time validators
			// are where such a value should be refused.
			if len(value) < minRedactableSecretLen {
				continue
			}
			s = strings.ReplaceAll(s, value, Redacted)
		}
	}
	return truncate(s)
}

func truncate(s string) string {
	if len(s) <= maxErrorLen {
		return s
	}
	return s[:maxErrorLen] + "…"
}

func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	obj, err := decodeObjectErr(raw, "config")
	if err != nil {
		return nil, false
	}
	return obj, true
}

func decodeObjectErr(raw json.RawMessage, what string) (map[string]json.RawMessage, error) {
	obj := make(map[string]json.RawMessage)
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return obj, nil
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, wrapErr("parse "+what, err)
	}
	return obj, nil
}

func nonEmptyString(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil && s != ""
}

func isEmptyString(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil && s == ""
}

func isNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}
