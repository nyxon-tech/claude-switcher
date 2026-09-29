//go:build darwin || linux

package platform

import (
	"os"
	"syscall"
)

// ownerUID is the real user whose processes count as Desktop: ours, because another user's
// Desktop (fast user switching, a shared machine) cannot have our data open and cannot be
// signalled by us. As root (sudo) it is -1, every user: the data may belong to anyone.
func ownerUID() int {
	if uid := os.Getuid(); uid != 0 {
		return uid
	}
	return -1
}

// signalDesktop sends sig to Desktop's main processes, or to all of them when onlyMain is false.
func signalDesktop(in Install, sig syscall.Signal, onlyMain bool) error {
	ps, err := desktopProcesses(in)
	if err != nil {
		return err
	}
	for _, p := range ps {
		if p.Main || !onlyMain {
			_ = syscall.Kill(p.PID, sig) // it may have exited already
		}
	}
	return nil
}
