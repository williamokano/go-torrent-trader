package service

import (
	"crypto/hmac"
	"crypto/md5"  // #nosec G501 -- building a legacy fixture
	"crypto/sha1" // #nosec G505 -- building a legacy fixture
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// #228. VerifyPassword accepts Argon2id and nothing else, so a member migrated from a
// PHP TorrentTrader could not log in *at all* — not "was asked to reset", but could
// not authenticate, because the legacy digest never parsed as an Argon2id encoding
// and verification failed before any comparison happened.
//
// Several documents claimed the opposite for a long time, which is the same shape as
// the JWT and the rate limiter the 2026-07-26 alignment pass found: a guarantee that
// was never the system's behaviour, cited downstream until somebody checked.
//
// The fixtures are built with the same primitives PHP's sha1(), md5() and
// hash_hmac() produce, so these are the values that will actually be in the column.
func TestALegacyMemberCanLogInAndIsUpgraded(t *testing.T) {
	const password = "correct horse battery staple"
	const secret = "site-secret-from-the-old-install"

	for _, tc := range []struct {
		scheme string
		stored string
	}{
		{scheme: SchemeLegacySHA1, stored: sha1Hex(password)},
		{scheme: SchemeLegacyMD5, stored: md5Hex(password)},
		{scheme: SchemeLegacyHMACSHA1, stored: hmacSHA1Hex(password, secret)},
	} {
		t.Run(tc.scheme, func(t *testing.T) {
			match, needsUpgrade, err := VerifyPasswordWithScheme(password, tc.stored, tc.scheme, secret)
			if err != nil {
				t.Fatalf("VerifyPasswordWithScheme: %v", err)
			}
			if !match {
				t.Error("the correct password was rejected, so a migrated member " +
					"cannot log in")
			}
			if !needsUpgrade {
				t.Error("needsUpgrade is false, so the member would stay on a weak " +
					"hash forever instead of upgrading on this login")
			}

			// And the wrong password is still wrong.
			bad, _, err := VerifyPasswordWithScheme("wrong", tc.stored, tc.scheme, secret)
			if err != nil {
				t.Fatalf("VerifyPasswordWithScheme(wrong): %v", err)
			}
			if bad {
				t.Error("an incorrect password was accepted")
			}
		})
	}
}

// The wrapped schemes hold argon2id(legacy_digest(password)), so the migration can
// re-hash at cutover and leave no bare SHA1 or MD5 at rest. The member still logs in
// with the password they always had.
func TestAWrappedLegacyHashVerifiesAndUpgrades(t *testing.T) {
	const password = "hunter2"
	const secret = "old-secret"

	for _, tc := range []struct {
		scheme string
		digest string
	}{
		{scheme: SchemeWrappedSHA1, digest: sha1Hex(password)},
		{scheme: SchemeWrappedMD5, digest: md5Hex(password)},
		{scheme: SchemeWrappedHMACSHA1, digest: hmacSHA1Hex(password, secret)},
	} {
		t.Run(tc.scheme, func(t *testing.T) {
			// What the migration would store: Argon2id over the legacy digest.
			wrapped, err := HashPassword(tc.digest)
			if err != nil {
				t.Fatalf("HashPassword: %v", err)
			}

			match, needsUpgrade, err := VerifyPasswordWithScheme(password, wrapped, tc.scheme, secret)
			if err != nil {
				t.Fatalf("VerifyPasswordWithScheme: %v", err)
			}
			if !match {
				t.Error("the correct password was rejected against a wrapped hash")
			}
			if !needsUpgrade {
				t.Error("a wrapped hash should still upgrade to plain argon2id on login")
			}

			bad, _, err := VerifyPasswordWithScheme("wrong", wrapped, tc.scheme, secret)
			if err != nil {
				t.Fatalf("VerifyPasswordWithScheme(wrong): %v", err)
			}
			if bad {
				t.Error("an incorrect password was accepted against a wrapped hash")
			}
		})
	}
}

// An Argon2id member must be unaffected, and must never be told to upgrade — a
// needless re-hash on every login would be a silent performance regression on the
// overwhelmingly common path.
func TestAnArgon2idMemberIsUnaffected(t *testing.T) {
	hashed, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	for _, scheme := range []string{SchemeArgon2id, "", "  ARGON2ID  "} {
		t.Run("scheme="+scheme, func(t *testing.T) {
			match, needsUpgrade, err := VerifyPasswordWithScheme("hunter2", hashed, scheme, "")
			if err != nil {
				t.Fatalf("VerifyPasswordWithScheme: %v", err)
			}
			if !match {
				t.Error("an argon2id member was rejected")
			}
			if needsUpgrade {
				t.Error("an argon2id member was marked for upgrade, which would " +
					"re-hash on every single login")
			}
		})
	}
}

// An HMAC member with no configured secret is an operator misconfiguration, and has
// to be distinguishable from a wrong password — otherwise every affected member is
// sent to the reset form with nothing explaining why.
func TestAMissingHMACSecretIsReportedNotTreatedAsABadPassword(t *testing.T) {
	stored := hmacSHA1Hex("hunter2", "the-secret")

	_, _, err := VerifyPasswordWithScheme("hunter2", stored, SchemeLegacyHMACSHA1, "")
	if !errors.Is(err, ErrLegacySecretMissing) {
		t.Errorf("err = %v, want ErrLegacySecretMissing", err)
	}
}

// A scheme nothing can verify must be an error rather than a silent false, which
// would be indistinguishable from a wrong password and would hide a bad migration.
func TestAnUnknownSchemeIsAnError(t *testing.T) {
	_, _, err := VerifyPasswordWithScheme("hunter2", "whatever", "bcrypt", "")
	if !errors.Is(err, ErrUnknownPasswordScheme) {
		t.Errorf("err = %v, want ErrUnknownPasswordScheme", err)
	}
}

// PHP emits lower-case hex, but a migration or a mod may have upper-cased it on the
// way through, and a member should not be locked out by the case of their own hash.
func TestALegacyDigestIsComparedCaseInsensitively(t *testing.T) {
	stored := strings.ToUpper(sha1Hex("hunter2"))

	match, _, err := VerifyPasswordWithScheme("hunter2", stored, SchemeLegacySHA1, "")
	if err != nil {
		t.Fatalf("VerifyPasswordWithScheme: %v", err)
	}
	if !match {
		t.Error("an upper-cased legacy digest rejected the correct password")
	}
}

// A truncated or malformed stored value must not match, and must not panic.
func TestAMalformedLegacyHashDoesNotMatch(t *testing.T) {
	for _, stored := range []string{"", "abc", strings.Repeat("f", 39), strings.Repeat("f", 41), "not-hex-at-all-but-40-characters-long!!!"} {
		match, _, err := VerifyPasswordWithScheme("hunter2", stored, SchemeLegacySHA1, "")
		if err != nil {
			t.Errorf("stored=%q: unexpected error %v", stored, err)
		}
		if match {
			t.Errorf("stored=%q matched", stored)
		}
	}
}

// The legacy schemes are inbound only. Nothing may create an account or change a
// password into one — that would be choosing a weak hash for a credential created
// today, rather than tolerating one inherited from a fifteen-year-old site.
//
// Asserted against the constants rather than the call sites so a scheme added later
// is covered without anyone remembering to extend a list.
func TestEveryLegacySchemeIsRecognisedAsLegacy(t *testing.T) {
	legacy := []string{
		SchemeLegacySHA1, SchemeLegacyMD5, SchemeLegacyHMACSHA1,
		SchemeWrappedSHA1, SchemeWrappedMD5, SchemeWrappedHMACSHA1,
	}
	for _, scheme := range legacy {
		if !IsLegacyScheme(scheme) {
			t.Errorf("IsLegacyScheme(%q) = false; a writer guarding on this would "+
				"happily store it", scheme)
		}
	}
	if IsLegacyScheme(SchemeArgon2id) {
		t.Error("IsLegacyScheme(argon2id) = true, which would block the only scheme " +
			"this system is allowed to write")
	}
}

// --- fixtures matching what PHP produces ------------------------------------

func sha1Hex(s string) string {
	sum := sha1.Sum([]byte(s)) // #nosec G401 -- fixture
	return hex.EncodeToString(sum[:])
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s)) // #nosec G401 -- fixture
	return hex.EncodeToString(sum[:])
}

func hmacSHA1Hex(s, secret string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(s))
	return hex.EncodeToString(mac.Sum(nil))
}
