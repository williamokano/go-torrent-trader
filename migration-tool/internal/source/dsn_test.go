package source

import (
	"strings"
	"testing"
)

func TestParseDSNURLForm(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "full URL",
			in:   "mysql://root:secret@db.example.com:3307/torrenttrader",
			want: "root:secret@tcp(db.example.com:3307)/torrenttrader",
		},
		{
			name: "port defaults to 3306",
			in:   "mysql://root:secret@db.example.com/torrenttrader",
			want: "root:secret@tcp(db.example.com:3306)/torrenttrader",
		},
		{
			name: "host defaults to loopback",
			in:   "mysql://root:secret@/torrenttrader",
			want: "root:secret@tcp(127.0.0.1:3306)/torrenttrader",
		},
		{
			name: "no password",
			in:   "mysql://root@localhost:3306/torrenttrader",
			want: "root@tcp(localhost:3306)/torrenttrader",
		},
		{
			name: "mariadb scheme",
			in:   "mariadb://root:secret@localhost:3306/torrenttrader",
			want: "root:secret@tcp(localhost:3306)/torrenttrader",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDSN(tc.in)
			if err != nil {
				t.Fatalf("ParseDSN(%q) returned %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseDSN(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A password from a real install is as likely as not to contain a character
// that means something in a DSN. Percent-decoding it is the whole reason the
// URL form goes through net/url rather than string splitting.
func TestParseDSNDecodesAwkwardPasswords(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantPass string
	}{
		{"at sign", "mysql://root:p%40ss@localhost:3306/tt", "p@ss"},
		{"slash", "mysql://root:p%2Fss@localhost:3306/tt", "p/ss"},
		{"colon", "mysql://root:p%3Ass@localhost:3306/tt", "p:ss"},
		{"question mark", "mysql://root:p%3Fss@localhost:3306/tt", "p?ss"},
		{"parenthesis", "mysql://root:p%29ss@localhost:3306/tt", "p)ss"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfig(tc.in)
			if err != nil {
				t.Fatalf("parseConfig(%q) returned %v", tc.in, err)
			}
			if cfg.Passwd != tc.wantPass {
				t.Errorf("password = %q, want %q", cfg.Passwd, tc.wantPass)
			}
		})
	}
}

func TestParseDSNKeepsQueryParameters(t *testing.T) {
	got, err := ParseDSN("mysql://root:secret@localhost:3306/tt?tls=skip-verify&readTimeout=30s")
	if err != nil {
		t.Fatalf("ParseDSN returned %v", err)
	}
	for _, want := range []string{"tls=skip-verify", "readTimeout=30s"} {
		if !strings.Contains(got, want) {
			t.Errorf("ParseDSN dropped %q; got %q", want, got)
		}
	}
}

func TestParseDSNAcceptsDriverForm(t *testing.T) {
	tests := []string{
		"root:secret@tcp(localhost:3306)/torrenttrader",
		"root:secret@unix(/var/run/mysqld/mysqld.sock)/torrenttrader",
		"root@tcp(127.0.0.1:3306)/torrenttrader?charset=utf8mb4",
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := ParseDSN(in)
			if err != nil {
				t.Fatalf("ParseDSN(%q) returned %v", in, err)
			}
			if !strings.Contains(got, "torrenttrader") {
				t.Errorf("ParseDSN(%q) = %q, lost the database name", in, got)
			}
		})
	}
}

func TestParseDSNRejectsUnusableInput(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"URL without a database", "mysql://root:secret@localhost:3306/"},
		{"driver form without a database", "root:secret@tcp(localhost:3306)/"},
		{"not a DSN at all", "just some text"},
		{"unknown parameter value", "mysql://root@localhost:3306/tt?tls=not-a-tls-mode"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseDSN(tc.in); err == nil {
				t.Errorf("ParseDSN(%q) returned no error", tc.in)
			}
		})
	}
}

// A DSN reaches this tool on a command line and leaves it in logs and error
// messages. The password must not make that trip.
func TestDescribeDSNHidesThePassword(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"URL form", "mysql://root:hunter2@db.example.com:3307/tt", "root:***@db.example.com:3307/tt"},
		{"driver form", "root:hunter2@tcp(localhost:3306)/tt", "root:***@localhost:3306/tt"},
		{"no password", "mysql://root@localhost:3306/tt", "root@localhost:3306/tt"},
		{"unparseable", "nonsense", "<unparseable DSN>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DescribeDSN(tc.in)
			if got != tc.want {
				t.Errorf("DescribeDSN(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, "hunter2") {
				t.Errorf("DescribeDSN leaked the password: %q", got)
			}
		})
	}
}

// url.Error embeds the URL it failed on, so wrapping it would put the password
// into an error message that gets logged.
func TestParseDSNErrorsDoNotLeakThePassword(t *testing.T) {
	// A control character makes url.Parse fail after the password is present.
	_, err := ParseDSN("mysql://root:hunter2@localhost:3306/tt\x7f\x00")
	if err == nil {
		t.Fatal("expected an error for a malformed URL")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error leaked the password: %v", err)
	}
}

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "plain", in: "users", want: "`users`"},
		{name: "underscore", in: "forum_posts", want: "`forum_posts`"},
		// A backtick is the one character that could end the quoted
		// identifier early, so it is doubled rather than passed through.
		{name: "backtick is escaped", in: "we`ird", want: "`we``ird`"},
		{name: "injection attempt", in: "users`; DROP TABLE users; --", want: "`users``; DROP TABLE users; --`"},
		{name: "empty", in: "", wantErr: true},
		{name: "NUL byte", in: "us\x00ers", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := quoteIdentifier(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("quoteIdentifier(%q) returned no error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("quoteIdentifier(%q) returned %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("quoteIdentifier(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
