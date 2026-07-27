package service

import (
	"crypto/hmac"
	"crypto/md5"  // #nosec G501 -- reading a legacy hash, never producing one
	"crypto/sha1" // #nosec G505 -- reading a legacy hash, never producing one
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
)

// Password schemes recorded on users.password_scheme.
//
// Only SchemeArgon2id is ever written. The rest exist so a member migrated from a
// PHP TorrentTrader can log in once with the password they already had, and be
// upgraded in the process — see VerifyPasswordWithScheme.
const (
	// SchemeArgon2id is what this system produces. Everything else is inbound only.
	SchemeArgon2id = "argon2id"

	// TorrentTrader 3.x hashed with whichever of these its passhash() was
	// configured for (FULL_FEATURE_DOCUMENTATION.md §4.1). All three are
	// unsalted or site-salted hex digests, which is why they must never survive
	// a successful login.
	SchemeLegacySHA1     = "legacy_sha1"
	SchemeLegacyMD5      = "legacy_md5"
	SchemeLegacyHMACSHA1 = "legacy_hmac_sha1"

	// The wrapped variants hold argon2id(legacy_digest(password)) — the migration
	// re-hashes at cutover so no bare SHA1 or MD5 is ever at rest in the new
	// database. Verification computes the legacy digest and feeds it to the
	// Argon2id verifier, so the member still logs in with their original password.
	//
	// This is the stronger position and the one an operator should prefer, at the
	// cost of one extra digest per login until the member upgrades. #159's
	// --wrap-passwords flag chooses between them; the backend has to be able to
	// verify both either way.
	SchemeWrappedSHA1     = "wrapped_sha1"
	SchemeWrappedMD5      = "wrapped_md5"
	SchemeWrappedHMACSHA1 = "wrapped_hmac_sha1"
)

// ErrLegacySecretMissing reports that a member's hash needs the legacy site secret
// and none is configured.
//
// Distinct from a wrong password on purpose: it is an operator misconfiguration, and
// treating it as bad credentials would send every HMAC-scheme member to the password
// reset form with nothing in the logs to explain why.
var ErrLegacySecretMissing = errors.New("legacy HMAC secret is not configured")

// ErrUnknownPasswordScheme reports a password_scheme value nothing can verify.
var ErrUnknownPasswordScheme = errors.New("unknown password scheme")

// legacyDigest computes the TorrentTrader-side digest for a scheme, or reports that
// the scheme is not a legacy one.
//
// Hex, lower case, because that is what PHP's sha1(), md5() and hash_hmac() return
// and what therefore sits in the migrated column.
func legacyDigest(scheme, password, secret string) (string, bool, error) {
	switch scheme {
	case SchemeLegacySHA1, SchemeWrappedSHA1:
		sum := sha1.Sum([]byte(password)) // #nosec G401 -- verifying an existing hash
		return hex.EncodeToString(sum[:]), true, nil

	case SchemeLegacyMD5, SchemeWrappedMD5:
		sum := md5.Sum([]byte(password)) // #nosec G401 -- verifying an existing hash
		return hex.EncodeToString(sum[:]), true, nil

	case SchemeLegacyHMACSHA1, SchemeWrappedHMACSHA1:
		if secret == "" {
			return "", true, ErrLegacySecretMissing
		}
		mac := hmac.New(sha1.New, []byte(secret))
		mac.Write([]byte(password))
		return hex.EncodeToString(mac.Sum(nil)), true, nil
	}
	return "", false, nil
}

// IsLegacyScheme reports whether a scheme is one this system only ever reads.
//
// Used to keep a legacy scheme unwritable: nothing may register an account or change
// a password into one, because that would be choosing a weak hash for a credential
// created today rather than tolerating one inherited from a fifteen-year-old site.
func IsLegacyScheme(scheme string) bool {
	_, isLegacy, _ := legacyDigest(scheme, "", "x")
	return isLegacy
}

// isWrappedScheme reports whether the stored value is an Argon2id encoding wrapping a
// legacy digest, rather than the bare digest itself.
func isWrappedScheme(scheme string) bool {
	switch scheme {
	case SchemeWrappedSHA1, SchemeWrappedMD5, SchemeWrappedHMACSHA1:
		return true
	}
	return false
}

// VerifyPasswordWithScheme checks a password against a stored hash, dispatching on
// the scheme that hash was written in.
//
// The reason this exists: VerifyPassword assumes Argon2id and returns ErrInvalidHash
// for anything else, so a migrated member could not log in at all — not "was asked to
// reset", but could not authenticate, because the legacy digest never parsed as an
// Argon2id encoding and verification failed before any comparison happened. Several
// documents claimed the opposite for a long time (#228).
//
// A true return with needsUpgrade set means the caller must re-hash the password with
// Argon2id and store it, so each migrated member upgrades exactly once, on their next
// login. The caller does that rather than this function, because it owns the
// repository and the transaction.
//
// An empty scheme is treated as Argon2id: rows predating the column, and every test
// fixture that builds a user without setting it, are Argon2id in practice.
func VerifyPasswordWithScheme(password, encodedHash, scheme, legacySecret string) (match bool, needsUpgrade bool, err error) {
	scheme = strings.TrimSpace(strings.ToLower(scheme))
	if scheme == "" {
		scheme = SchemeArgon2id
	}

	if scheme == SchemeArgon2id {
		ok, verifyErr := VerifyPassword(password, encodedHash)
		return ok, false, verifyErr
	}

	digest, isLegacy, digestErr := legacyDigest(scheme, password, legacySecret)
	if digestErr != nil {
		return false, false, digestErr
	}
	if !isLegacy {
		return false, false, ErrUnknownPasswordScheme
	}

	// Wrapped: the column holds argon2id(digest), so the digest is the "password"
	// the Argon2id verifier is given. No weak hash is at rest in that case.
	if isWrappedScheme(scheme) {
		ok, verifyErr := VerifyPassword(digest, encodedHash)
		if verifyErr != nil {
			return false, false, verifyErr
		}
		return ok, ok, nil
	}

	// Bare: the column holds the hex digest itself. Compared in constant time and
	// case-insensitively, since PHP emits lower case but a migration or a mod may
	// have upper-cased it on the way through.
	stored := strings.TrimSpace(strings.ToLower(encodedHash))
	if len(stored) != len(digest) {
		// Length is not secret — it is fixed per scheme — so returning early here
		// leaks nothing a caller could not compute from the scheme name.
		return false, false, nil
	}
	ok := subtle.ConstantTimeCompare([]byte(stored), []byte(digest)) == 1
	return ok, ok, nil
}
