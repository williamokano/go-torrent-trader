package source

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// defaultPort is MySQL's default TCP port, used when a URL-form DSN omits one.
const defaultPort = "3306"

// ParseDSN accepts either of the two spellings an operator is likely to have to
// hand and returns a DSN the MySQL driver understands.
//
// The URL form is what this project's README and .env examples use:
//
//	mysql://user:password@host:3306/torrenttrader?tls=skip-verify
//
// The driver's own form is accepted unchanged, so a DSN copied from existing
// tooling keeps working:
//
//	user:password@tcp(host:3306)/torrenttrader
//	user:password@unix(/var/run/mysqld/mysqld.sock)/torrenttrader
//
// A database name is required either way: every query the tool runs is scoped to
// one schema, and a DSN without one fails later and less clearly.
func ParseDSN(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("no source DSN: pass --source or set MIGRATION_SOURCE_DSN")
	}

	cfg, err := parseConfig(raw)
	if err != nil {
		return "", err
	}
	if cfg.DBName == "" {
		return "", errors.New("source DSN has no database name")
	}
	return cfg.FormatDSN(), nil
}

func parseConfig(raw string) (*mysql.Config, error) {
	if isURLForm(raw) {
		return parseURLDSN(raw)
	}
	// Errors from the driver describe the grammar, not the value, so they are
	// safe to surface — the DSN itself must never appear in output.
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		return nil, fmt.Errorf("source DSN is not valid: %w", err)
	}
	return cfg, nil
}

func isURLForm(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "mysql://") || strings.HasPrefix(lower, "mariadb://")
}

// parseURLDSN converts the URL spelling into a driver config. Parameters are
// handed to the driver rather than copied field by field, so anything it
// supports — tls, timeouts, charset — works without this function knowing about
// it.
func parseURLDSN(raw string) (*mysql.Config, error) {
	u, err := url.Parse(raw)
	if err != nil {
		// url.Error embeds the URL, which carries the password. Do not wrap it.
		return nil, errors.New("source DSN is not a valid URL")
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	if strings.Contains(dbName, "/") {
		return nil, fmt.Errorf("source DSN path %q is not a database name", "/"+dbName)
	}

	// Build the parameter-only part and let the driver parse it, so an unknown
	// or malformed parameter is rejected here rather than at connect time.
	spec := "/" + dbName
	if u.RawQuery != "" {
		spec += "?" + u.RawQuery
	}
	cfg, err := mysql.ParseDSN(spec)
	if err != nil {
		return nil, fmt.Errorf("source DSN parameters are not valid: %w", err)
	}

	host := u.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := u.Port()
	if port == "" {
		port = defaultPort
	}

	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(host, port)
	if u.User != nil {
		cfg.User = u.User.Username()
		// Percent-escapes in the password are decoded here, which is the
		// reason for going through net/url rather than string surgery.
		if pass, ok := u.User.Password(); ok {
			cfg.Passwd = pass
		}
	}
	return cfg, nil
}

// DescribeDSN renders a DSN for logs and error messages with the password
// removed. Nothing in this tool prints a DSN any other way.
func DescribeDSN(raw string) string {
	cfg, err := parseConfig(raw)
	if err != nil {
		return "<unparseable DSN>"
	}
	var b strings.Builder
	if cfg.User != "" {
		b.WriteString(cfg.User)
		if cfg.Passwd != "" {
			b.WriteString(":***")
		}
		b.WriteString("@")
	}
	addr := cfg.Addr
	if addr == "" {
		addr = "127.0.0.1:" + defaultPort
	}
	b.WriteString(addr)
	b.WriteString("/")
	b.WriteString(cfg.DBName)
	return b.String()
}
