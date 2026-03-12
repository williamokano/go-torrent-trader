package postgres

import (
	"strings"
	"testing"
)

func TestBuildPrefixQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ascii words",
			input: "hello world",
			want:  "hello:* & world:*",
		},
		{
			name:  "single word",
			input: "test",
			want:  "test:*",
		},
		{
			name:  "unicode CJK",
			input: "日本語",
			want:  "日本語:*",
		},
		{
			name:  "accented characters",
			input: "café résumé",
			want:  "café:* & résumé:*",
		},
		{
			name:  "mixed ASCII and CJK",
			input: "hello 世界",
			want:  "hello:* & 世界:*",
		},
		{
			name:  "special chars stripped within word",
			input: "hello@world!",
			want:  "helloworld:*",
		},
		{
			name:  "special chars stripped between words",
			input: "hello@ world!",
			want:  "hello:* & world:*",
		},
		{
			name:  "only special chars",
			input: "@#$%",
			want:  "",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "digits",
			input: "abc123 456",
			want:  "abc123:* & 456:*",
		},
		{
			name:  "extra whitespace",
			input: "  hello   world  ",
			want:  "hello:* & world:*",
		},
		{
			name:  "single special char word among valid words",
			input: "hello @ world",
			want:  "hello:* & world:*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildPrefixQuery(tt.input)
			if got != tt.want {
				t.Errorf("BuildPrefixQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildPrefixQuery_LongInput(t *testing.T) {
	words := make([]string, 100)
	for i := range words {
		words[i] = "word"
	}
	input := strings.Join(words, " ")
	got := BuildPrefixQuery(input)
	if got == "" {
		t.Error("BuildPrefixQuery with 100 words returned empty string")
	}
	// Should have 100 terms joined by " & "
	parts := strings.Split(got, " & ")
	if len(parts) != 100 {
		t.Errorf("expected 100 parts, got %d", len(parts))
	}
	for _, p := range parts {
		if p != "word:*" {
			t.Errorf("unexpected part: %q", p)
		}
	}
}
