//go:build windows

package config

// modeChecksSupported is false on Windows, where the Unix permission bits this
// package inspects do not exist.
//
// Go synthesizes a mode rather than reporting one: os.Stat returns 0666 for a
// writable file and 0444 for a read-only one (os/types_windows.go). Both trip a
// `mode&0o077 != 0` test, so the credential check would reject every file it ever
// wrote — and os.Chmod there only toggles the read-only attribute, so the advice
// to "chmod 600" names something the platform cannot do. The release workflow
// builds a windows/amd64 `tt`, so without this the binary would store a token
// once (the file does not exist yet, so the check is skipped) and then fail every
// authenticated command afterwards, permanently, with unfollowable advice.
//
// Skipping the check is not a silent downgrade of a security control: the control
// was never available here. Windows ACLs are the equivalent mechanism and are a
// separate piece of work — the config directory lives under %AppData%, which is
// already per-user by default.
const modeChecksSupported = false
