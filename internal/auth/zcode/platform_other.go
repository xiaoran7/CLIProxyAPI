//go:build !windows && !linux && !darwin

package zcode

// osRelease has no portable source on this platform; callers fall back to the
// GOOS name.
func osRelease() string { return "" }
