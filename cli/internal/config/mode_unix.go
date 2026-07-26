//go:build !windows

package config

// modeChecksSupported reports whether the filesystem's permission bits mean what
// this package reads them to mean.
//
// True everywhere except Windows; see mode_windows.go for why that matters.
const modeChecksSupported = true
