package mention

import (
	"reflect"
	"testing"
)

func TestExtract(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"none", "just some text", []string{}},
		{"start of string", "@alice hi", []string{"alice"}},
		{"after whitespace", "hi @bob", []string{"bob"}},
		{"after newline", "line one\n@carol", []string{"carol"}},
		{"after open paren", "(@dave)", []string{"dave"}},
		{"multiple", "hey @bob and @carol", []string{"bob", "carol"}},
		{"duplicates preserved", "@bob @bob", []string{"bob", "bob"}},
		// An email address is not a mention: the @ must start the string or follow
		// whitespace / an open paren, never a word character.
		{"email is not a mention", "mail me at alice@bob.example", []string{}},
		{"mid-word is not a mention", "foo@bar", []string{}},
		{"underscores and digits", "@user_42", []string{"user_42"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Extract(tc.body)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Extract(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
