package testenv_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/testenv"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Cleanup()
	os.Exit(code)
}

// #225. Every test in this module runs against one corpus, and the corpus is worth
// what its awkward rows are worth: "a million clean rows would prove less than the
// fifty awkward ones", as its own header says. A migration does not fail on ordinary
// data.
//
// Nothing enforced that. The awkward rows were documented in a comment and present by
// habit, so anything that regenerated, tidied or "fixed" the file — making it load
// under strict mode, for instance, which is what would remove half of them — could
// take them out and leave every test in the module green. The suite would keep
// passing and simply stop proving anything, which is the failure mode this repository
// has been bitten by repeatedly.
//
// So each property is asserted against the corpus **as MySQL actually stored it**,
// not by grepping the file. That distinction matters: a zero date in the file only
// counts if MySQL kept it as one rather than coercing it, and the coercion is exactly
// what the sql_mode line at the top of the corpus exists to prevent.
func TestTheCorpusStillCarriesItsAdversarialRows(t *testing.T) {
	db, err := sql.Open("mysql", testenv.Legacy(t))
	if err != nil {
		t.Fatalf("connecting to the legacy corpus: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, tc := range []struct {
		name  string
		query string
		why   string
	}{{
		name:  "a zero date",
		query: "SELECT COUNT(*) FROM users WHERE CAST(added AS CHAR) LIKE '0000-00-00%'",
		why: "MyISAM accepts '0000-00-00 00:00:00' and PostgreSQL rejects it outright, " +
			"so this is the row that proves dates are converted rather than copied",
	}, {
		name:  "the invited_by = 0 sentinel",
		query: "SELECT COUNT(*) FROM users WHERE invited_by = 0",
		why: "the legacy \"nobody\" value meeting a real foreign key — it has to become " +
			"NULL, and 0 would point at no user",
	}, {
		name:  "an over-long username",
		query: "SELECT COUNT(*) FROM users WHERE CHAR_LENGTH(username) > 20",
		why: "legacy varchar(40) into the target's varchar(20); truncating silently " +
			"collides two members into one",
	}, {
		name:  "a duplicate passkey",
		query: "SELECT COUNT(*) FROM (SELECT secret FROM users GROUP BY secret HAVING COUNT(*) > 1) d",
		why:   "UNIQUE in the target and not in MyISAM, so the second insert aborts the users load",
	}, {
		name:  "a dangling class",
		query: "SELECT COUNT(*) FROM users u LEFT JOIN `groups` g ON g.group_id = u.class WHERE g.group_id IS NULL",
		why:   "a member whose group was deleted years ago; the class cannot resolve to a group id",
	}, {
		name:  "a malformed info hash",
		query: "SELECT COUNT(*) FROM torrents WHERE CHAR_LENGTH(info_hash) <> 40",
		why: "not 40 hex characters, so hex decoding fails — and info_hash is BYTEA NOT " +
			"NULL UNIQUE in the target, with no recovery from that side",
	}, {
		name:  "an orphaned peer",
		query: "SELECT COUNT(*) FROM peers p LEFT JOIN users u ON u.id = p.userid WHERE u.id IS NULL",
		why:   "a peer whose member was deleted; the foreign key has nowhere to point",
	}, {
		name:  "an orphaned completion",
		query: "SELECT COUNT(*) FROM completed c LEFT JOIN torrents t ON t.id = c.torrentid WHERE t.id IS NULL",
		why:   "the same, on the other side of the snatch list",
	}, {
		name:  "latin1 high bytes",
		query: "SELECT COUNT(*) FROM users WHERE HEX(username) REGEXP '[8-9A-F][0-9A-F]' OR HEX(email) REGEXP '[8-9A-F][0-9A-F]'",
		why: "the only proof the encoding is converted rather than copied — a latin1 é " +
			"is one byte and a UTF-8 é is two",
	}, {
		name:  "a torrent with no file rows",
		query: "SELECT COUNT(*) FROM torrents t LEFT JOIN files f ON f.torrent = t.id WHERE f.id IS NULL",
		why:   "a torrent whose file list was never recorded; the transformer must not assume one exists",
	}, {
		name:  "unclosed or nested BBCode",
		query: "SELECT COUNT(*) FROM forum_posts WHERE body LIKE '%[%'",
		why:   "the converter's hard cases, and the reason it is a tokenizer rather than a regexp",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			var n int64
			if err := db.QueryRowContext(context.Background(), tc.query).Scan(&n); err != nil {
				t.Fatalf("querying the corpus: %v", err)
			}
			if n == 0 {
				t.Errorf("the corpus no longer contains %s.\n%s\n\nIf this row was "+
					"removed deliberately, the migration stopped being tested against "+
					"it — put it back, or say in the corpus header why it is gone.",
					tc.name, tc.why)
			}
		})
	}
}

// The structural deviations matter as much as the rows: they are what the schema
// comparison is checked against. A corpus regenerated from a stock TorrentTrader
// would drop every one of them and quietly reduce `validate` to a no-op.
func TestTheCorpusStillCarriesItsStructuralDeviations(t *testing.T) {
	db, err := sql.Open("mysql", testenv.Legacy(t))
	if err != nil {
		t.Fatalf("connecting to the legacy corpus: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const inSchema = "TABLE_SCHEMA = DATABASE()"

	for _, tc := range []struct {
		name  string
		query string
		why   string
	}{{
		name: "a mod-added column",
		query: "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE " + inSchema +
			" AND TABLE_NAME = 'users' AND COLUMN_NAME IN ('seedbonus','karma')",
		why: "columns no stock install has; the mapping must offer them for a decision " +
			"rather than dropping them",
	}, {
		name: "a mod-added table",
		query: "SELECT COUNT(*) FROM information_schema.TABLES WHERE " + inSchema +
			" AND TABLE_NAME = 'bonus_log'",
		why: "a whole table the reference does not know about",
	}, {
		name: "a latin1 table",
		query: "SELECT COUNT(*) FROM information_schema.TABLES WHERE " + inSchema +
			" AND TABLE_COLLATION LIKE 'latin1%'",
		why: "what a 2008-era install actually is, and the reason the tool reports " +
			"encodings at all — a UTF-8 corpus would never exercise the conversion",
	}, {
		name: "a stock table this install dropped",
		query: "SELECT (SELECT COUNT(*) FROM information_schema.TABLES WHERE " + inSchema +
			" AND TABLE_NAME = 'polls') = 0",
		why: "the absent-table branch of the comparison; an install that dropped one " +
			"must still migrate",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			var n int64
			if err := db.QueryRowContext(context.Background(), tc.query).Scan(&n); err != nil {
				t.Fatalf("querying the corpus: %v", err)
			}
			if n == 0 {
				t.Errorf("the corpus no longer has %s.\n%s", tc.name, tc.why)
			}
		})
	}
}
