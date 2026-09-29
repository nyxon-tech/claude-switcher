//go:build !windows

package ops

// platformChecks has nothing to add outside Windows.
func platformChecks(*Env) []Check { return nil }
