package connector

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DefaultRatePerMin bounds how many announcements one instance may send per
// minute when it does not say otherwise. Anything beyond the budget is folded
// into a single "+N more" summary rather than dropped.
const DefaultRatePerMin = 20

// RatePerMinKey is a config key every kind understands. It is parsed by the
// drain handler rather than by each connector, which is also why config decoding
// has to tolerate keys a connector's own struct does not declare.
const RatePerMinKey = "rate_per_min"

// DecodeConfig unmarshals an instance config into v.
//
// Unknown keys are tolerated on purpose. Every config carries the shared
// rate_per_min key that no connector's own struct declares, and a row written by
// a newer build may carry keys this one has never heard of — neither should make
// an existing instance undeliverable. Bad *values* are still caught, by each
// connector's ValidateConfig at save time.
func DecodeConfig(raw json.RawMessage, v any) error {
	if isBlankJSON(raw) {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	return nil
}

// RatePerMin reads the shared rate_per_min key, falling back to the default for
// an absent, unparseable or non-positive value. A misconfigured 0 must not mean
// "never send anything".
func RatePerMin(raw json.RawMessage) int {
	var cfg struct {
		RatePerMin *int `json:"rate_per_min"`
	}
	if err := DecodeConfig(raw, &cfg); err != nil || cfg.RatePerMin == nil || *cfg.RatePerMin <= 0 {
		return DefaultRatePerMin
	}
	return *cfg.RatePerMin
}

// ValidateRatePerMin rejects a nonsensical shared rate limit at save time.
func ValidateRatePerMin(raw json.RawMessage) error {
	var cfg struct {
		RatePerMin *int `json:"rate_per_min"`
	}
	if err := DecodeConfig(raw, &cfg); err != nil {
		return err
	}
	if cfg.RatePerMin != nil && *cfg.RatePerMin <= 0 {
		return fmt.Errorf("%s must be a positive number of messages per minute", RatePerMinKey)
	}
	return nil
}

func isBlankJSON(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}

// wrapErr keeps error wrapping uniform across the package's small helpers.
func wrapErr(what string, err error) error {
	return fmt.Errorf("%s: %w", what, err)
}
